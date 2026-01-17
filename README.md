# NPWS Package

[![Go Reference](https://pkg.go.dev/badge/github.com/NodePassProject/npws.svg)](https://pkg.go.dev/github.com/NodePassProject/npws)
[![License](https://img.shields.io/badge/License-BSD_3--Clause-blue.svg)](https://opensource.org/licenses/BSD-3-Clause)

A high-performance, reliable WebSocket connection pool management system for Go applications.

## Table of Contents

- [Features](#features)
- [Installation](#installation)
- [Quick Start](#quick-start)
- [Usage](#usage)
  - [Client Connection Pool](#client-connection-pool)
  - [Server Connection Pool](#server-connection-pool)
  - [Managing Pool Health](#managing-pool-health)
- [Security Features](#security-features)
  - [Client IP Restriction](#client-ip-restriction)
  - [TLS Security Modes](#tls-security-modes)
- [Dynamic Adjustment](#dynamic-adjustment)
- [Advanced Usage](#advanced-usage)
- [Performance Considerations](#performance-considerations)
- [Troubleshooting](#troubleshooting)
- [Best Practices](#best-practices)
  - [Pool Configuration](#1-pool-configuration)
  - [Connection Management](#2-connection-management)
  - [Error Handling and Monitoring](#3-error-handling-and-monitoring)
  - [Production Deployment](#4-production-deployment)
  - [Performance Optimization](#5-performance-optimization)
  - [Testing and Development](#6-testing-and-development)
- [License](#license)

## Features

- **Lock-free design** using atomic operations and `sync.Map` for maximum performance
- **Thread-safe connection management** with automatic lifecycle handling
- **Support for both client and server connection pools**
- **Dynamic capacity and interval adjustment** based on real-time usage patterns
- **Automatic connection health monitoring** with keep-alive support
- **WebSocket connection pooling** with efficient ID-based retrieval
- **4-byte hex connection identification** for efficient tracking
- **Multiple TLS security modes** (self-signed, verified)
- **Graceful error handling and recovery** with automatic retry mechanisms
- **Configurable connection creation intervals** with dynamic adjustment
- **Auto-reconnection** on connection failures
- **Built-in keep-alive management** with configurable periods
- **Zero lock contention** for high concurrency scenarios
- **Client IP whitelisting** for server-side security

## Installation

```bash
go get github.com/NodePassProject/npws
```

## Quick Start

Here's a minimal example to get you started:

```go
package main

import (
    "log"
    "time"
    "github.com/NodePassProject/npws"
)

func main() {
    // Create client pool
    minCap := 5
    maxCap := 20
    minInterval := 500 * time.Millisecond
    maxInterval := 5 * time.Second
    keepAlive := 30 * time.Second
    tlsMode := "2" // TLS mode: "1" = self-signed, "2" = verified
    serverURL := "example.com:8443"

    clientPool := npws.NewClientPool(
        minCap, maxCap,
        minInterval, maxInterval,
        keepAlive,
        tlsMode,
        serverURL,
    )
    defer clientPool.Close()

    // Start client manager
    go clientPool.ClientManager()

    // Wait for pool to be ready
    time.Sleep(2 * time.Second)

    // Get connection by ID (received from server)
    timeout := 10 * time.Second
    conn, err := clientPool.OutgoingGet("a1b2c3d4", timeout)
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()

    // Use connection...
}
```

## Usage

### Client Connection Pool

```go
package main

import (
    "log"
    "time"
    "github.com/NodePassProject/npws"
)

func main() {
    // Configure client pool parameters
    minCap := 5              // Minimum pool capacity
    maxCap := 20             // Maximum pool capacity
    minInterval := 500 * time.Millisecond  // Minimum creation interval
    maxInterval := 5 * time.Second         // Maximum creation interval
    keepAlive := 30 * time.Second          // Keep-alive period
    tlsMode := "2"           // TLS mode: "1" = self-signed, "2" = verified
    serverURL := "example.com:8443"        // Server address (host:port)

    // Create client pool
    clientPool := npws.NewClientPool(
        minCap, maxCap,
        minInterval, maxInterval,
        keepAlive,
        tlsMode,
        serverURL,
    )
    defer clientPool.Close()

    // Start pool manager in background
    go clientPool.ClientManager()

    // Wait for pool initialization
    time.Sleep(2 * time.Second)

    // Check pool status
    if !clientPool.Ready() {
        log.Fatal("Pool not ready")
    }

    log.Printf("Pool active connections: %d", clientPool.Active())
    log.Printf("Pool capacity: %d", clientPool.Capacity())
    log.Printf("Pool interval: %v", clientPool.Interval())

    // Get connection by ID with timeout
    timeout := 10 * time.Second
    conn, err := clientPool.OutgoingGet("a1b2c3d4", timeout)
    if err != nil {
        log.Printf("Connection not found: %v", err)
        return
    }
    defer conn.Close()

    // Use connection for communication
    _, err = conn.Write([]byte("Hello, Server!"))
    if err != nil {
        log.Printf("Write error: %v", err)
        return
    }

    buf := make([]byte, 1024)
    n, err := conn.Read(buf)
    if err != nil {
        log.Printf("Read error: %v", err)
        return
    }
    log.Printf("Received: %s", string(buf[:n]))
}
```

**Note:** `OutgoingGet` takes a connection ID and timeout duration, and returns `(net.Conn, error)`. 
The error indicates if the connection with the specified ID was not found or if the timeout was exceeded.

### Server Connection Pool

```go
package main

import (
    "crypto/tls"
    "log"
    "net"
    "time"
    "github.com/NodePassProject/npws"
)

func main() {
    // Load TLS certificate
    cert, err := tls.LoadX509KeyPair("server.crt", "server.key")
    if err != nil {
        log.Fatal(err)
    }

    tlsConfig := &tls.Config{
        Certificates: []tls.Certificate{cert},
        MinVersion:   tls.VersionTLS13,
    }

    // Create TCP listener
    listener, err := net.Listen("tcp", "0.0.0.0:8443")
    if err != nil {
        log.Fatal(err)
    }

    // Configure server pool
    maxCap := 100                      // Maximum pool capacity
    clientIP := ""                     // Client IP whitelist ("" = allow all)
    keepAlive := 30 * time.Second      // Keep-alive period

    // Create server pool
    serverPool := npws.NewServerPool(
        maxCap,
        clientIP,
        tlsConfig,
        listener,
        keepAlive,
    )
    if serverPool == nil {
        log.Fatal("Failed to create server pool")
    }
    defer serverPool.Close()

    // Start server manager in background
    go serverPool.ServerManager()

    log.Printf("Server listening on %s", listener.Addr())

    // Accept connections from pool
    for {
        timeout := 30 * time.Second
        id, conn, err := serverPool.IncomingGet(timeout)
        if err != nil {
            log.Printf("Failed to get connection: %v", err)
            continue
        }

        log.Printf("New connection: %s", id)
        go handleConnection(id, conn)
    }
}

func handleConnection(id string, conn net.Conn) {
    defer conn.Close()
    
    buf := make([]byte, 1024)
    n, err := conn.Read(buf)
    if err != nil {
        log.Printf("Read error: %v", err)
        return
    }
    
    log.Printf("Connection %s received: %s", id, string(buf[:n]))
    
    // Echo back
    _, err = conn.Write(buf[:n])
    if err != nil {
        log.Printf("Write error: %v", err)
    }
}
```

**Note:** `IncomingGet` takes a timeout duration and returns `(string, net.Conn, error)`. The return values are:
- `string`: The connection ID generated by the server (8-character hex string)
- `net.Conn`: The connection object (wrapped WebSocket connection)
- `error`: Can indicate timeout, context cancellation, or other pool-related errors

### Managing Pool Health

```go
// Get a connection from client pool by ID with timeout
timeout := 10 * time.Second
conn, err := clientPool.OutgoingGet("a1b2c3d4", timeout)
if err != nil {
    // Connection with the specified ID not found or timeout exceeded
    log.Printf("Connection not found: %v", err)
}

// Get a connection from server pool with timeout
timeout := 30 * time.Second
id, conn, err := serverPool.IncomingGet(timeout)
if err != nil {
    // Handle various error cases:
    // - Pool is full or no connections available
    // - Context cancelled
    // - Timeout exceeded
    log.Printf("Failed to get connection: %v", err)
}

// Check if the pool is ready
if clientPool.Ready() {
    // The pool is initialized and ready for use
}

// Get current active connection count
activeConns := clientPool.Active()

// Get current capacity setting
capacity := clientPool.Capacity()

// Get current connection creation interval
interval := clientPool.Interval()

// Manually flush all connections (rarely needed, closes all connections)
clientPool.Flush()

// Record an error (increases internal error counter)
clientPool.AddError()

// Get the current error count
errorCount := clientPool.ErrorCount()

// Reset the error count to zero
clientPool.ResetError()
```

## Security Features

### Client IP Restriction

The `NewServerPool` function allows you to restrict incoming connections to a specific client IP address. The function signature is:

```go
func NewServerPool(
    maxCap int,
    clientIP string,
    tlsConfig *tls.Config,
    listener net.Listener,
    keepAlive time.Duration,
) *Pool
```

- `maxCap`: Maximum pool capacity.
- `clientIP`: Restrict allowed client IP ("" for any).
- `tlsConfig`: TLS configuration (required for secure connections).
- `listener`: Network listener (TCP listener).
- `keepAlive`: Keep-alive period.

When the `clientIP` parameter is set:
- All connections from other IP addresses will be immediately rejected with HTTP 403.
- This provides an additional layer of security beyond network firewalls.
- Particularly useful for internal services or dedicated client-server applications.

To allow connections from any IP address, use an empty string:

```go
// Create a server pool that accepts connections from any IP
serverPool := npws.NewServerPool(
    100, 
    "", 
    tlsConfig, 
    listener, 
    30*time.Second,
)
```

To restrict to a specific client IP:

```go
// Create a server pool that only accepts connections from 192.168.1.100
serverPool := npws.NewServerPool(
    100, 
    "192.168.1.100", 
    tlsConfig, 
    listener, 
    30*time.Second,
)
```

### TLS Security Modes

| Mode | Description | Security Level | Use Case |
|------|-------------|----------------|----------|
| `"1"` | Self-signed certificates (InsecureSkipVerify) | Medium | Development, testing environments |
| `"2"` | Verified certificates | High | Production, public networks |

**Note:** Both modes `"1"` and `"2"` use secure WebSocket (`wss://`) with TLS encryption.

#### Example Usage

```go
// Self-signed TLS - development/testing (InsecureSkipVerify)
clientPool := npws.NewClientPool(
    5, 20, 
    500*time.Millisecond, 5*time.Second, 
    30*time.Second, 
    "1", 
    "example.com:8443",
)

// Verified TLS - production
clientPool := npws.NewClientPool(
    5, 20, 
    500*time.Millisecond, 5*time.Second, 
    30*time.Second, 
    "2", 
    "example.com:8443",
)
```

---

**Implementation Details:**

- **Connection ID Generation:**
  - The server generates a 4-byte random ID using `crypto/rand`
  - Encoded as 8-character hexadecimal string (e.g., "a1b2c3d4")
  - IDs are unique within the pool to prevent collisions
  - Sent to client immediately after WebSocket handshake

- **OutgoingGet Method:**
  - For client pools: Returns `(net.Conn, error)` after retrieving a connection by ID.
  - Takes timeout parameter to wait for connection availability.
  - Removes connection from pool upon successful retrieval.

- **IncomingGet Method:**
  - For server pools: Returns `(string, net.Conn, error)` to get an available connection with its ID.
  - Blocks until a connection is available or timeout occurs.
  - Returns the connection ID and connection object for further use.

- **Flush/Close:**
  - `Flush` closes all connections and resets the pool.
  - `Close` cancels the context, shuts down HTTP server, and flushes the pool.

- **Dynamic Adjustment:**
  - `adjustInterval` and `adjustCapacity` are used internally for pool optimization based on usage and success rate.

- **Error Handling:**
  - `AddError`, `ErrorCount`, and `ResetError` are thread-safe using atomic operations.

- **WSConn Wrapper:**
  - Wraps WebSocket connection to implement `net.Conn` interface.
  - Provides `ConnectionState()` method for TLS connection state inspection.
  - Preserves local and remote address information.

## Dynamic Adjustment

The pool automatically adjusts parameters based on real-time metrics:

### Interval Adjustment (per creation cycle)
- **Decreases interval** (faster creation) when idle connections < 20% of capacity
  - Adjustment: `interval = max(interval - 100ms, minInterval)`
- **Increases interval** (slower creation) when idle connections > 80% of capacity
  - Adjustment: `interval = min(interval + 100ms, maxInterval)`

### Capacity Adjustment (after each creation attempt)
- **Decreases capacity** when success rate < 20%
  - Adjustment: `capacity = max(capacity - 1, minCapacity)`
- **Increases capacity** when success rate > 80%
  - Adjustment: `capacity = min(capacity + 1, maxCapacity)`

Monitor adjustments:

```go
// Check current settings
currentCapacity := clientPool.Capacity()   // Current target capacity
currentInterval := clientPool.Interval()   // Current creation interval
activeConns := clientPool.Active()         // Available connections

// Calculate utilization
utilization := float64(activeConns) / float64(currentCapacity)
log.Printf("Pool: %d/%d connections (%.1f%%), %v interval",
    activeConns, currentCapacity, utilization*100, currentInterval)
```

## Advanced Usage

### Custom Error Handling

```go
package main

import (
    "log"
    "time"
    "github.com/NodePassProject/npws"
)

func main() {
    clientPool := npws.NewClientPool(
        5, 20,
        500*time.Millisecond, 5*time.Second,
        30*time.Second,
        "2",
        "example.com:8443",
    )
    defer clientPool.Close()

    go clientPool.ClientManager()

    // Monitor error rate
    go func() {
        ticker := time.NewTicker(10 * time.Second)
        defer ticker.Stop()
        
        for range ticker.C {
            errorCount := clientPool.ErrorCount()
            if errorCount > 10 {
                log.Printf("High error rate detected: %d errors", errorCount)
                clientPool.ResetError()
                
                // Take action: restart pool or alert
                clientPool.Flush()
            }
        }
    }()

    // Your application logic...
    time.Sleep(60 * time.Second)
}
```

### Working with Context

```go
package main

import (
    "context"
    "log"
    "time"
    "github.com/NodePassProject/npws"
)

func main() {
    // Create a context that can be cancelled
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    clientPool := npws.NewClientPool(
        5, 20,
        500*time.Millisecond, 5*time.Second,
        30*time.Second,
        "1",
        "example.com:8443",
    )

    // Start manager
    go clientPool.ClientManager()

    // Graceful shutdown on signal
    go func() {
        <-ctx.Done()
        log.Println("Shutting down pool...")
        clientPool.Close()
    }()

    // Application runs until context is cancelled
    <-ctx.Done()
}
```

### Load Balancing with Multiple Pools

```go
package main

import (
    "sync/atomic"
    "time"
    "github.com/NodePassProject/npws"
)

func main() {
    // Create pools for different servers
    pools := []*npws.Pool{
        npws.NewClientPool(5, 20, 500*time.Millisecond, 5*time.Second, 30*time.Second, "2", "server1.example.com:8443"),
        npws.NewClientPool(5, 20, 500*time.Millisecond, 5*time.Second, 30*time.Second, "2", "server2.example.com:8443"),
        npws.NewClientPool(5, 20, 500*time.Millisecond, 5*time.Second, 30*time.Second, "2", "server3.example.com:8443"),
    }

    // Start all managers
    for _, pool := range pools {
        go pool.ClientManager()
        defer pool.Close()
    }

    // Round-robin load balancing
    var counter atomic.Uint32
    getNextPool := func() *npws.Pool {
        idx := counter.Add(1) % uint32(len(pools))
        return pools[idx]
    }

    // Use pools in round-robin fashion
    for i := 0; i < 10; i++ {
        pool := getNextPool()
        
        conn, err := pool.OutgoingGet("some-id", 5*time.Second)
        if err != nil {
            pool.AddError()
            continue
        }
        
        // Use connection...
        conn.Close()
    }
}
```

## Performance Considerations

### Lock-Free Architecture

This package uses a **lock-free design** for maximum concurrency:

| Component | Implementation | Benefit |
|-----------|----------------|---------|
| **Connection Storage** | `sync.Map` | Lock-free concurrent access |
| **Counters** | `atomic.Int32` / `atomic.Int64` | Lock-free increments |
| **ID Channel** | Buffered `chan string` | Native Go concurrency |

**Performance Impact:**
- Zero lock contention in high-concurrency scenarios
- No context switching overhead from mutex waits
- Scales linearly with CPU cores
- Consistent sub-microsecond operation latency

### Connection Pool Sizing

| Pool Size | Pros | Cons | Best For |
|-----------|------|------|----------|
| Too Small (< 5) | Low resource usage | Connection contention, delays | Low-traffic applications |
| Optimal (5-50) | Balanced performance | Requires monitoring | Most applications |
| Too Large (> 100) | No contention | Resource waste, memory overhead | Very high-traffic services |

**Sizing Guidelines:**
- Start with `minCap = baseline_load` and `maxCap = peak_load × 1.5`
- Monitor connection usage with `pool.Active()` and `pool.Capacity()`
- Adjust based on observed patterns

### WebSocket Performance Impact

| Aspect | WebSocket | Plain TCP |
|--------|-----------|-----------|
| **Handshake Time** | ~50-100ms (HTTP upgrade) | ~10-20ms (direct) |
| **Protocol Overhead** | Minimal (frame headers) | None |
| **Browser Compatibility** | Excellent | Limited |
| **Firewall Traversal** | Easy (HTTP ports) | May be blocked |
| **Throughput** | ~95% of TCP | Baseline |

### Connection Creation Overhead

Connection creation in WebSocket involves:
- **Cost**: ~50-100ms per connection (HTTP upgrade + TLS handshake)
- **Frequency**: Controlled by pool intervals
- **Trade-off**: Fast creation vs. resource usage

For ultra-high-throughput systems, consider pre-creating connections during idle periods.

## Troubleshooting

### Common Issues

#### 1. Connection Timeout
**Symptoms:** WebSocket connection fails to establish  
**Solutions:**
- Check network connectivity to target host
- Verify server address and port are correct
- Ensure firewall allows WebSocket connections
- Check for proxy/NAT issues

#### 2. TLS Handshake Failure
**Symptoms:** Connections fail with certificate errors  
**Solutions:**
- Verify certificate validity and expiration
- Check hostname matches certificate Common Name
- Ensure TLS 1.3 is supported
- For testing, temporarily use TLS mode `"1"` (InsecureSkipVerify)

#### 3. Pool Exhaustion
**Symptoms:** `IncomingGet()` returns timeout error  
**Solutions:**
- Check WebSocket server status
- Increase maximum capacity
- Reduce connection hold time in application code
- Check for connection leaks (ensure connections are properly closed)
- Monitor with `pool.Active()`, `pool.Capacity()`
- Use appropriate timeout values with `IncomingGet(timeout)`

#### 4. High Error Rate
**Symptoms:** Frequent connection creation failures  
**Solutions:**
- Check WebSocket server stability
- Monitor network connectivity
- Verify server is accepting connections
- Track errors with `pool.AddError()` and `pool.ErrorCount()`

#### 5. HTTP Server Closed Error
**Symptoms:** Server manager stops with "http: Server closed"  
**Solutions:**
- Normal behavior when `Close()` is called
- Check context cancellation
- Review graceful shutdown procedures

### Debugging Checklist

- [ ] **Network connectivity**: Can you reach the target host?
- [ ] **Port accessibility**: Is the WebSocket port open and not blocked?
- [ ] **Certificate validity**: For TLS, are certificates valid and not expired?
- [ ] **Pool capacity**: Is `maxCap` sufficient for your load?
- [ ] **Connection leaks**: Are you properly closing connections?
- [ ] **Error monitoring**: Are you tracking `pool.ErrorCount()`?
- [ ] **Firewall/Proxy**: Are there WebSocket-specific restrictions?

### Debug Logging

Add logging at key points for better debugging:

```go
// Log successful connection retrieval
id, conn, err := serverPool.IncomingGet(30 * time.Second)
if err != nil {
    log.Printf("Connection retrieval failed: %v", err)
    serverPool.AddError()
} else {
    log.Printf("Connection retrieved successfully: %s", id)
}

// Monitor pool health
ticker := time.NewTicker(10 * time.Second)
go func() {
    for range ticker.C {
        log.Printf("Pool status - Active: %d, Capacity: %d, Interval: %v, Errors: %d",
            pool.Active(), pool.Capacity(), pool.Interval(), pool.ErrorCount())
    }
}()
```

## Best Practices

### 1. Pool Configuration

#### Capacity Sizing
```go
// For most applications, start with these guidelines:
minCap := expectedConcurrentConnections
maxCap := peakConcurrentConnections * 1.5

// Example for a web service handling 50 concurrent connections
clientPool := npws.NewClientPool(
    50, 75,                             // min/max capacity
    500*time.Millisecond,               // min interval
    5*time.Second,                      // max interval
    30*time.Second,                     // keep-alive
    "2",                                // verified TLS
    "api.example.com:8443",             // server address
)

// For high-traffic API handling 200 concurrent connections
highTrafficPool := npws.NewClientPool(
    150, 300,                           // min/max capacity
    200*time.Millisecond,               // min interval (faster)
    3*time.Second,                      // max interval
    30*time.Second,                     // keep-alive
    "2",                                // verified TLS
    "api.example.com:8443",             // server address
)

log.Printf("Pool created with capacity %d-%d", minCap, maxCap)
```

#### Interval Configuration
```go
// Aggressive (high-frequency applications)
minInterval := 100 * time.Millisecond
maxInterval := 1 * time.Second

// Balanced (general purpose)
minInterval := 500 * time.Millisecond
maxInterval := 5 * time.Second

// Conservative (low-frequency, batch processing)
minInterval := 2 * time.Second
maxInterval := 10 * time.Second
```

#### Leverage Lock-Free Architecture
```go
// The lock-free design allows safe concurrent access to pool metrics
// No need to worry about mutex contention or race conditions

func monitorPoolMetrics(pool *npws.Pool) {
    ticker := time.NewTicker(1 * time.Second)
    defer ticker.Stop()
    
    for range ticker.C {
        // All these operations are lock-free and thread-safe
        active := pool.Active()
        capacity := pool.Capacity()
        interval := pool.Interval()
        errors := pool.ErrorCount()
        
        log.Printf("Pool metrics - Active: %d, Cap: %d, Interval: %v, Errors: %d",
            active, capacity, interval, errors)
    }
}
```

### 2. Connection Management

#### Always Close Connections
```go
// GOOD: Always close connections
id, conn, err := serverPool.IncomingGet(30 * time.Second)
if err != nil {
    log.Printf("Failed to get connection: %v", err)
    return err
}
if conn != nil {
    defer conn.Close()  // Close the connection when done
    // Use connection...
}

// BAD: Forgetting to close connections leads to resource leaks
id, conn, _ := serverPool.IncomingGet(30 * time.Second)
// Missing Close() - causes resource leak!
```

#### Handle Timeouts Gracefully
```go
// Use reasonable timeouts for IncomingGet
timeout := 10 * time.Second
id, conn, err := serverPool.IncomingGet(timeout)
if err != nil {
    log.Printf("Connection timeout: %v", err)
    serverPool.AddError()
    return err
}
if conn == nil {
    log.Printf("Unexpected nil connection")
    return errors.New("unexpected nil connection")
}
defer conn.Close()
```

### 3. Error Handling and Monitoring

#### Implement Comprehensive Error Tracking
```go
type PoolManager struct {
    pool        *npws.Pool
    maxErrors   int
    logger      *log.Logger
}

func (pm *PoolManager) getConnectionWithRetry(id string, maxRetries int) (net.Conn, error) {
    for i := 0; i < maxRetries; i++ {
        conn, err := pm.pool.OutgoingGet(id, 5*time.Second)
        if err == nil {
            return conn, nil
        }
        
        pm.pool.AddError()
        pm.logger.Printf("Retry %d/%d: %v", i+1, maxRetries, err)
        time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
    }
    
    return nil, errors.New("max retries exceeded")
}

// Monitor pool health periodically
func (pm *PoolManager) healthCheck() {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    
    for range ticker.C {
        errorCount := pm.pool.ErrorCount()
        active := pm.pool.Active()
        capacity := pm.pool.Capacity()
        
        if errorCount > pm.maxErrors {
            pm.logger.Printf("High error rate: %d errors", errorCount)
            pm.pool.Flush()
            pm.pool.ResetError()
        }
        
        utilization := float64(active) / float64(capacity) * 100
        pm.logger.Printf("Pool health - Utilization: %.1f%%, Errors: %d",
            utilization, errorCount)
    }
}
```

### 4. Production Deployment

#### Security Configuration
```go
// Production server setup with proper TLS
func createProductionServer() (*npws.Pool, error) {
    // Load production certificates
    cert, err := tls.LoadX509KeyPair("server.crt", "server.key")
    if err != nil {
        return nil, err
    }
    
    tlsConfig := &tls.Config{
        Certificates: []tls.Certificate{cert},
        MinVersion:   tls.VersionTLS13,
        CipherSuites: []uint16{
            tls.TLS_AES_128_GCM_SHA256,
            tls.TLS_AES_256_GCM_SHA384,
            tls.TLS_CHACHA20_POLY1305_SHA256,
        },
    }
    
    listener, err := net.Listen("tcp", "0.0.0.0:8443")
    if err != nil {
        return nil, err
    }
    
    // Restrict to specific client IP for security
    return npws.NewServerPool(
        100,
        "192.168.1.100", // Only allow this client
        tlsConfig,
        listener,
        30*time.Second,
    ), nil
}

// Client with verified certificates
func createProductionClient() *npws.Pool {
    return npws.NewClientPool(
        20, 50,
        500*time.Millisecond,
        5*time.Second,
        30*time.Second,
        "2", // Verified TLS
        "api.example.com:8443",
    )
}
```

#### Graceful Shutdown
```go
func (app *Application) Shutdown(ctx context.Context) error {
    // Stop accepting new requests first
    app.isShuttingDown.Store(true)
    
    // Give existing connections time to complete
    shutdownTimeout := 10 * time.Second
    shutdownCtx, cancel := context.WithTimeout(ctx, shutdownTimeout)
    defer cancel()
    
    // Close all pools
    for _, pool := range app.pools {
        pool.Close()
    }
    
    <-shutdownCtx.Done()
    return nil
}
```

### 5. Performance Optimization

#### Avoid Common Anti-patterns
```go
// ANTI-PATTERN: Creating pools repeatedly
func badHandler(w http.ResponseWriter, r *http.Request) {
    // DON'T: Create a new pool for each request
    pool := npws.NewClientPool(5, 20, 500*time.Millisecond, 5*time.Second, 30*time.Second, "2", "server:8443")
    defer pool.Close()
}

// GOOD PATTERN: Reuse pools
type Server struct {
    wsPool *npws.Pool // Shared pool instance
}

func (s *Server) goodHandler(w http.ResponseWriter, r *http.Request) {
    // DO: Reuse existing pool
    conn, err := s.wsPool.OutgoingGet("conn-id", 5*time.Second)
    if err != nil {
        http.Error(w, err.Error(), http.StatusServiceUnavailable)
        return
    }
    defer conn.Close()
    
    // Handle request...
}
```

#### Optimize for Your Use Case
```go
// High-throughput, low-latency services
highThroughputPool := npws.NewClientPool(
    100, 200,                           // Large capacity
    100*time.Millisecond,               // Fast creation
    1*time.Second,                      // Quick adjustment
    15*time.Second,                     // Short keep-alive
    "2",
    "api.example.com:8443",
)

// Long-running, stable connections
stablePool := npws.NewClientPool(
    10, 20,                             // Smaller capacity
    1*time.Second,                      // Slower creation
    10*time.Second,                     // Longer adjustment
    60*time.Second,                     // Long keep-alive
    "2",
    "backend.example.com:8443",
)

// Batch processing with bursts
batchPool := npws.NewClientPool(
    5, 50,                              // Wide capacity range
    2*time.Second,                      // Slow baseline
    10*time.Second,                     // Wide adjustment range
    30*time.Second,                     // Standard keep-alive
    "2",
    "batch.example.com:8443",
)
```

### 6. Testing and Development

#### Use Development Mode
```go
// Development setup with relaxed security
func createDevPool() *npws.Pool {
    return npws.NewClientPool(
        2, 5,                           // Small capacity for testing
        1*time.Second,                  // Slow intervals for observation
        5*time.Second,
        10*time.Second,                 // Short keep-alive
        "1",                            // Self-signed certs (InsecureSkipVerify)
        "localhost:8443",
    )
}
```

#### Integration Testing
```go
func TestPoolIntegration(t *testing.T) {
    // Setup server
    cert, err := tls.LoadX509KeyPair("testdata/server.crt", "testdata/server.key")
    if err != nil {
        t.Fatal(err)
    }
    
    tlsConfig := &tls.Config{
        Certificates: []tls.Certificate{cert},
        MinVersion:   tls.VersionTLS13,
    }
    
    listener, err := net.Listen("tcp", "127.0.0.1:0")
    if err != nil {
        t.Fatal(err)
    }
    
    serverPool := npws.NewServerPool(10, "", tlsConfig, listener, 10*time.Second)
    go serverPool.ServerManager()
    defer serverPool.Close()
    
    // Setup client
    clientPool := npws.NewClientPool(
        2, 5,
        500*time.Millisecond, 2*time.Second,
        10*time.Second,
        "1",  // Self-signed certs for testing
        listener.Addr().String(),
    )
    go clientPool.ClientManager()
    defer clientPool.Close()
    
    // Wait for pool initialization
    time.Sleep(2 * time.Second)
    
    // Test connection flow
    id, conn, err := serverPool.IncomingGet(5 * time.Second)
    if err != nil {
        t.Fatalf("Failed to get connection: %v", err)
    }
    defer conn.Close()
    
    t.Logf("Connection established with ID: %s", id)
}
```

## License

This project is licensed under the BSD 3-Clause License.  
See the [LICENSE](LICENSE) file for details.