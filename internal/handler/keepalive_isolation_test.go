package handler

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestKeepAliveContextIsolation_SequentialRequests verifies that sequential requests
// on the same Keep-Alive connection do NOT leak context data between requests.
// This is a security test to ensure request isolation (Issue #29).
func TestKeepAliveContextIsolation_SequentialRequests(t *testing.T) {
	// Track received headers for each request
	var mu sync.Mutex
	receivedRequests := make([]map[string]string, 0)

	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		headers := map[string]string{
			"X-Request-Id":     r.Header.Get("X-Request-Id"),
			"X-Tenant-Id":      r.Header.Get("X-Tenant-Id"),
			"X-Correlation-Id": r.Header.Get("X-Correlation-Id"),
		}
		receivedRequests = append(receivedRequests, headers)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer targetServer.Close()

	targetHost := targetServer.Listener.Addr().String()
	cfg := testConfig(targetHost, []string{"x-request-id", "x-tenant-id", "x-correlation-id"})

	handler, err := NewProxyHandler(cfg)
	require.NoError(t, err)

	// Create a test server with Keep-Alive enabled
	proxyServer := httptest.NewServer(handler)
	defer proxyServer.Close()

	// Create a client that reuses connections (Keep-Alive enabled by default)
	client := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        1,
			MaxIdleConnsPerHost: 1,
			IdleConnTimeout:     30 * time.Second,
			DisableKeepAlives:   false, // Explicitly enable Keep-Alive
		},
	}
	defer client.CloseIdleConnections()

	// Request A: Send with specific headers
	reqA, err := http.NewRequest(http.MethodGet, proxyServer.URL+"/path-a", nil)
	require.NoError(t, err)
	reqA.Header.Set("X-Request-Id", "request-a-id")
	reqA.Header.Set("X-Tenant-Id", "tenant-a")
	reqA.Header.Set("X-Correlation-Id", "corr-a")

	respA, err := client.Do(reqA)
	require.NoError(t, err)
	respA.Body.Close()
	assert.Equal(t, http.StatusOK, respA.StatusCode)

	// Request B: Send with DIFFERENT headers on SAME connection
	reqB, err := http.NewRequest(http.MethodGet, proxyServer.URL+"/path-b", nil)
	require.NoError(t, err)
	reqB.Header.Set("X-Request-Id", "request-b-id")
	reqB.Header.Set("X-Tenant-Id", "tenant-b")
	// Intentionally NOT setting X-Correlation-Id

	respB, err := client.Do(reqB)
	require.NoError(t, err)
	respB.Body.Close()
	assert.Equal(t, http.StatusOK, respB.StatusCode)

	// Request C: Send with only one header
	reqC, err := http.NewRequest(http.MethodGet, proxyServer.URL+"/path-c", nil)
	require.NoError(t, err)
	reqC.Header.Set("X-Request-Id", "request-c-id")
	// NOT setting X-Tenant-Id or X-Correlation-Id

	respC, err := client.Do(reqC)
	require.NoError(t, err)
	respC.Body.Close()
	assert.Equal(t, http.StatusOK, respC.StatusCode)

	// Verify context isolation
	mu.Lock()
	defer mu.Unlock()

	require.Len(t, receivedRequests, 3, "Should have received 3 requests")

	// Request A should have all its headers
	assert.Equal(t, "request-a-id", receivedRequests[0]["X-Request-Id"], "Request A should have its own request ID")
	assert.Equal(t, "tenant-a", receivedRequests[0]["X-Tenant-Id"], "Request A should have its own tenant ID")
	assert.Equal(t, "corr-a", receivedRequests[0]["X-Correlation-Id"], "Request A should have its own correlation ID")

	// Request B should have its own headers, NOT Request A's
	assert.Equal(t, "request-b-id", receivedRequests[1]["X-Request-Id"], "Request B should have its own request ID")
	assert.Equal(t, "tenant-b", receivedRequests[1]["X-Tenant-Id"], "Request B should have its own tenant ID")
	assert.Empty(t, receivedRequests[1]["X-Correlation-Id"], "Request B should NOT have Request A's correlation ID (context leak!)")

	// Request C should only have its own header
	assert.Equal(t, "request-c-id", receivedRequests[2]["X-Request-Id"], "Request C should have its own request ID")
	assert.Empty(t, receivedRequests[2]["X-Tenant-Id"], "Request C should NOT have previous tenant IDs (context leak!)")
	assert.Empty(t, receivedRequests[2]["X-Correlation-Id"], "Request C should NOT have previous correlation IDs (context leak!)")
}

// TestKeepAliveContextIsolation_ConcurrentRequests verifies that concurrent requests
// on Keep-Alive connections do NOT interfere with each other.
func TestKeepAliveContextIsolation_ConcurrentRequests(t *testing.T) {
	const numRequests = 100

	// Track received headers for each request, keyed by request ID
	var mu sync.Mutex
	receivedHeaders := make(map[string]map[string]string)

	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-Id")
		mu.Lock()
		receivedHeaders[requestID] = map[string]string{
			"X-Request-Id": requestID,
			"X-Tenant-Id":  r.Header.Get("X-Tenant-Id"),
			"X-User-Id":    r.Header.Get("X-User-Id"),
		}
		mu.Unlock()
		// Add small delay to increase chance of race conditions
		time.Sleep(time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer targetServer.Close()

	targetHost := targetServer.Listener.Addr().String()
	cfg := testConfig(targetHost, []string{"x-request-id", "x-tenant-id", "x-user-id"})

	handler, err := NewProxyHandler(cfg)
	require.NoError(t, err)

	proxyServer := httptest.NewServer(handler)
	defer proxyServer.Close()

	// Create client with connection pooling
	client := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     30 * time.Second,
			DisableKeepAlives:   false,
		},
	}
	defer client.CloseIdleConnections()

	// Send concurrent requests
	var wg sync.WaitGroup
	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			requestID := fmt.Sprintf("req-%d", idx)
			tenantID := fmt.Sprintf("tenant-%d", idx)
			userID := fmt.Sprintf("user-%d", idx)

			req, err := http.NewRequest(http.MethodGet, proxyServer.URL+"/test", nil)
			if err != nil {
				t.Errorf("Failed to create request %d: %v", idx, err)
				return
			}
			req.Header.Set("X-Request-Id", requestID)
			req.Header.Set("X-Tenant-Id", tenantID)
			req.Header.Set("X-User-Id", userID)

			resp, err := client.Do(req)
			if err != nil {
				t.Errorf("Request %d failed: %v", idx, err)
				return
			}
			resp.Body.Close()
		}(i)
	}

	wg.Wait()

	// Verify each request received its own headers (no cross-contamination)
	mu.Lock()
	defer mu.Unlock()

	assert.Len(t, receivedHeaders, numRequests, "Should have received all requests")

	for i := 0; i < numRequests; i++ {
		expectedRequestID := fmt.Sprintf("req-%d", i)
		expectedTenantID := fmt.Sprintf("tenant-%d", i)
		expectedUserID := fmt.Sprintf("user-%d", i)

		received, exists := receivedHeaders[expectedRequestID]
		if !exists {
			t.Errorf("Request %d not found in received headers", i)
			continue
		}

		assert.Equal(t, expectedTenantID, received["X-Tenant-Id"],
			"Request %d: tenant ID mismatch - possible context leak!", i)
		assert.Equal(t, expectedUserID, received["X-User-Id"],
			"Request %d: user ID mismatch - possible context leak!", i)
	}
}

// TestKeepAliveContextIsolation_RawTCPConnection tests context isolation using raw TCP
// to ensure we're actually reusing the same connection.
func TestKeepAliveContextIsolation_RawTCPConnection(t *testing.T) {
	var mu sync.Mutex
	receivedRequests := make([]map[string]string, 0)

	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedRequests = append(receivedRequests, map[string]string{
			"X-Request-Id": r.Header.Get("X-Request-Id"),
			"X-Tenant-Id":  r.Header.Get("X-Tenant-Id"),
		})
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer targetServer.Close()

	targetHost := targetServer.Listener.Addr().String()
	cfg := testConfig(targetHost, []string{"x-request-id", "x-tenant-id"})

	handler, err := NewProxyHandler(cfg)
	require.NoError(t, err)

	// Create test server without TLS for raw TCP testing
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	server := &http.Server{Handler: handler}
	go server.Serve(listener)
	defer server.Close()

	// Connect using raw TCP to ensure same connection
	conn, err := net.Dial("tcp", listener.Addr().String())
	require.NoError(t, err)
	defer conn.Close()

	// Send Request 1
	request1 := "GET /path1 HTTP/1.1\r\n" +
		"Host: localhost\r\n" +
		"Connection: keep-alive\r\n" +
		"X-Request-Id: raw-req-1\r\n" +
		"X-Tenant-Id: raw-tenant-1\r\n" +
		"\r\n"
	_, err = conn.Write([]byte(request1))
	require.NoError(t, err)

	// Read response 1
	buf := make([]byte, 4096)
	_, err = conn.Read(buf)
	require.NoError(t, err)

	// Send Request 2 on SAME connection
	request2 := "GET /path2 HTTP/1.1\r\n" +
		"Host: localhost\r\n" +
		"Connection: keep-alive\r\n" +
		"X-Request-Id: raw-req-2\r\n" +
		"\r\n" // Note: NOT sending X-Tenant-Id
	_, err = conn.Write([]byte(request2))
	require.NoError(t, err)

	// Read response 2
	_, err = conn.Read(buf)
	require.NoError(t, err)

	// Wait for requests to be processed
	time.Sleep(100 * time.Millisecond)

	// Verify isolation
	mu.Lock()
	defer mu.Unlock()

	require.Len(t, receivedRequests, 2, "Should have received 2 requests")

	// Request 1 should have both headers
	assert.Equal(t, "raw-req-1", receivedRequests[0]["X-Request-Id"])
	assert.Equal(t, "raw-tenant-1", receivedRequests[0]["X-Tenant-Id"])

	// Request 2 should NOT have Request 1's tenant ID
	assert.Equal(t, "raw-req-2", receivedRequests[1]["X-Request-Id"])
	assert.Empty(t, receivedRequests[1]["X-Tenant-Id"],
		"Context leak detected! Request 2 should not have Request 1's X-Tenant-Id")
}

// TestKeepAliveContextIsolation_GlobalStateNotShared verifies that handler-level
// state doesn't leak between requests.
func TestKeepAliveContextIsolation_GlobalStateNotShared(t *testing.T) {
	var requestCount atomic.Int32

	// Each request should see its own headers, even though they share the same handler
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := requestCount.Add(1)
		// Verify the request has its expected header value
		expectedValue := fmt.Sprintf("value-%d", count)
		actualValue := r.Header.Get("X-Request-Id")

		if actualValue != expectedValue {
			t.Errorf("Request %d: expected X-Request-Id=%s, got=%s", count, expectedValue, actualValue)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer targetServer.Close()

	targetHost := targetServer.Listener.Addr().String()
	cfg := testConfig(targetHost, []string{"x-request-id"})

	handler, err := NewProxyHandler(cfg)
	require.NoError(t, err)

	proxyServer := httptest.NewServer(handler)
	defer proxyServer.Close()

	client := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        1,
			MaxIdleConnsPerHost: 1,
			DisableKeepAlives:   false,
		},
	}
	defer client.CloseIdleConnections()

	// Send 10 sequential requests
	for i := 1; i <= 10; i++ {
		req, err := http.NewRequest(http.MethodGet, proxyServer.URL+"/test", nil)
		require.NoError(t, err)
		req.Header.Set("X-Request-Id", fmt.Sprintf("value-%d", i))

		resp, err := client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	}

	assert.Equal(t, int32(10), requestCount.Load(), "Should have processed 10 requests")
}

// TestKeepAliveContextIsolation_TransportRoundTrip verifies the HeaderPropagatingTransport
// doesn't share state between RoundTrip calls.
func TestKeepAliveContextIsolation_TransportRoundTrip(t *testing.T) {
	var mu sync.Mutex
	receivedHeaders := make([]map[string]string, 0)

	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedHeaders = append(receivedHeaders, map[string]string{
			"X-Request-Id": r.Header.Get("X-Request-Id"),
			"X-Tenant-Id":  r.Header.Get("X-Tenant-Id"),
		})
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer targetServer.Close()

	// Create transport with Keep-Alive
	baseTransport := &http.Transport{
		MaxIdleConns:        1,
		MaxIdleConnsPerHost: 1,
		IdleConnTimeout:     30 * time.Second,
		DisableKeepAlives:   false,
	}

	transport := NewHeaderPropagatingTransport(
		[]string{"x-request-id", "x-tenant-id"},
		baseTransport,
	)

	// Create client with our transport
	client := &http.Client{Transport: transport}
	defer client.CloseIdleConnections()

	// Request 1 with context containing headers
	req1, err := http.NewRequest(http.MethodGet, targetServer.URL+"/test1", nil)
	require.NoError(t, err)
	ctx1 := contextWithHeaders(map[string]string{
		"X-Request-Id": "transport-req-1",
		"X-Tenant-Id":  "transport-tenant-1",
	})
	req1 = req1.WithContext(ctx1)

	resp1, err := client.Do(req1)
	require.NoError(t, err)
	io.Copy(io.Discard, resp1.Body)
	resp1.Body.Close()

	// Request 2 with DIFFERENT context (no X-Tenant-Id)
	req2, err := http.NewRequest(http.MethodGet, targetServer.URL+"/test2", nil)
	require.NoError(t, err)
	ctx2 := contextWithHeaders(map[string]string{
		"X-Request-Id": "transport-req-2",
		// No X-Tenant-Id
	})
	req2 = req2.WithContext(ctx2)

	resp2, err := client.Do(req2)
	require.NoError(t, err)
	io.Copy(io.Discard, resp2.Body)
	resp2.Body.Close()

	// Verify isolation
	mu.Lock()
	defer mu.Unlock()

	require.Len(t, receivedHeaders, 2)

	// Request 1 should have both headers
	assert.Equal(t, "transport-req-1", receivedHeaders[0]["X-Request-Id"])
	assert.Equal(t, "transport-tenant-1", receivedHeaders[0]["X-Tenant-Id"])

	// Request 2 should NOT have Request 1's tenant ID
	assert.Equal(t, "transport-req-2", receivedHeaders[1]["X-Request-Id"])
	assert.Empty(t, receivedHeaders[1]["X-Tenant-Id"],
		"Transport context leak! Request 2 inherited Request 1's X-Tenant-Id")
}

// TestKeepAliveContextIsolation_RapidSequential sends rapid sequential requests
// to stress test context isolation.
func TestKeepAliveContextIsolation_RapidSequential(t *testing.T) {
	const numRequests = 1000

	var mu sync.Mutex
	headerMismatches := 0

	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-Id")
		tenantID := r.Header.Get("X-Tenant-Id")

		// Extract expected tenant from request ID (they should match)
		// e.g., "req-42" should have "tenant-42"
		var expectedNum int
		fmt.Sscanf(requestID, "req-%d", &expectedNum)
		expectedTenant := fmt.Sprintf("tenant-%d", expectedNum)

		if tenantID != expectedTenant {
			mu.Lock()
			headerMismatches++
			t.Logf("MISMATCH: requestID=%s, tenantID=%s, expected=%s", requestID, tenantID, expectedTenant)
			mu.Unlock()
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer targetServer.Close()

	targetHost := targetServer.Listener.Addr().String()
	cfg := testConfig(targetHost, []string{"x-request-id", "x-tenant-id"})

	handler, err := NewProxyHandler(cfg)
	require.NoError(t, err)

	proxyServer := httptest.NewServer(handler)
	defer proxyServer.Close()

	client := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        1,
			MaxIdleConnsPerHost: 1,
			DisableKeepAlives:   false,
		},
	}
	defer client.CloseIdleConnections()

	// Rapid fire sequential requests
	for i := 0; i < numRequests; i++ {
		req, err := http.NewRequest(http.MethodGet, proxyServer.URL+"/test", nil)
		require.NoError(t, err)
		req.Header.Set("X-Request-Id", fmt.Sprintf("req-%d", i))
		req.Header.Set("X-Tenant-Id", fmt.Sprintf("tenant-%d", i))

		resp, err := client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
	}

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 0, headerMismatches,
		"Found %d header mismatches indicating context leakage!", headerMismatches)
}

// TestKeepAliveContextIsolation_HTTP11Pipelining tests context isolation with
// HTTP/1.1 pipelining where multiple requests are sent before responses are read.
func TestKeepAliveContextIsolation_HTTP11Pipelining(t *testing.T) {
	var mu sync.Mutex
	receivedRequests := make([]map[string]string, 0)

	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedRequests = append(receivedRequests, map[string]string{
			"X-Request-Id": r.Header.Get("X-Request-Id"),
			"X-Tenant-Id":  r.Header.Get("X-Tenant-Id"),
			"Path":         r.URL.Path,
		})
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK\r\n"))
	}))
	defer targetServer.Close()

	targetHost := targetServer.Listener.Addr().String()
	cfg := testConfig(targetHost, []string{"x-request-id", "x-tenant-id"})

	handler, err := NewProxyHandler(cfg)
	require.NoError(t, err)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()

	server := &http.Server{Handler: handler}
	go server.Serve(listener)
	defer server.Close()

	// Connect using raw TCP for pipelining
	conn, err := net.Dial("tcp", listener.Addr().String())
	require.NoError(t, err)
	defer conn.Close()

	// Send multiple pipelined requests before reading responses
	pipelinedRequests := "" +
		"GET /req1 HTTP/1.1\r\n" +
		"Host: localhost\r\n" +
		"X-Request-Id: pipe-1\r\n" +
		"X-Tenant-Id: tenant-pipe-1\r\n" +
		"\r\n" +
		"GET /req2 HTTP/1.1\r\n" +
		"Host: localhost\r\n" +
		"X-Request-Id: pipe-2\r\n" +
		"X-Tenant-Id: tenant-pipe-2\r\n" +
		"\r\n" +
		"GET /req3 HTTP/1.1\r\n" +
		"Host: localhost\r\n" +
		"X-Request-Id: pipe-3\r\n" +
		"\r\n" // No tenant ID for req3

	_, err = conn.Write([]byte(pipelinedRequests))
	require.NoError(t, err)

	// Read all responses
	buf := make([]byte, 8192)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	totalRead := 0
	for {
		n, err := conn.Read(buf[totalRead:])
		if err != nil {
			break
		}
		totalRead += n
		// Check if we have received all 3 responses
		response := string(buf[:totalRead])
		if countOccurrences(response, "HTTP/1.1 200") >= 3 {
			break
		}
	}

	// Wait for all requests to be processed
	time.Sleep(200 * time.Millisecond)

	// Verify isolation
	mu.Lock()
	defer mu.Unlock()

	require.Len(t, receivedRequests, 3, "Should have received 3 pipelined requests")

	// Each request should have its own headers
	assert.Equal(t, "pipe-1", receivedRequests[0]["X-Request-Id"])
	assert.Equal(t, "tenant-pipe-1", receivedRequests[0]["X-Tenant-Id"])

	assert.Equal(t, "pipe-2", receivedRequests[1]["X-Request-Id"])
	assert.Equal(t, "tenant-pipe-2", receivedRequests[1]["X-Tenant-Id"])

	assert.Equal(t, "pipe-3", receivedRequests[2]["X-Request-Id"])
	assert.Empty(t, receivedRequests[2]["X-Tenant-Id"],
		"Pipelining context leak! Request 3 should not have previous tenant IDs")
}

// Helper function to count substring occurrences
func countOccurrences(s, substr string) int {
	count := 0
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			count++
		}
	}
	return count
}

// Helper function to create a context with headers
func contextWithHeaders(headers map[string]string) interface {
	Done() <-chan struct{}
	Err() error
	Value(key interface{}) interface{}
	Deadline() (deadline time.Time, ok bool)
} {
	return &testContext{headers: headers}
}

type testContext struct {
	headers map[string]string
}

func (c *testContext) Deadline() (deadline time.Time, ok bool) { return time.Time{}, false }
func (c *testContext) Done() <-chan struct{}                   { return nil }
func (c *testContext) Err() error                              { return nil }
func (c *testContext) Value(key interface{}) interface{} {
	if key == ContextKeyHeaders {
		return c.headers
	}
	return nil
}
