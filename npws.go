// Package npws 实现了基于WebSocket的高性能连接池管理系统
package npws

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

const (
	defaultMinCap           = 1
	defaultMaxCap           = 1
	defaultMinIvl           = 1 * time.Second
	defaultMaxIvl           = 1 * time.Second
	idReadTimeout           = 1 * time.Minute
	idRetryInterval         = 50 * time.Millisecond
	intervalAdjustStep      = 100 * time.Millisecond
	capacityAdjustLowRatio  = 0.2
	capacityAdjustHighRatio = 0.8
	intervalLowThreshold    = 0.2
	intervalHighThreshold   = 0.8
	wsPath                  = "/"
)

// WSConn WebSocket连接包装器
type WSConn struct {
	net.Conn
	tlsState   *tls.ConnectionState
	localAddr  net.Addr
	remoteAddr net.Addr
}

// ConnectionState 返回TLS连接状态
func (w *WSConn) ConnectionState() tls.ConnectionState {
	if w.tlsState != nil {
		return *w.tlsState
	}
	return tls.ConnectionState{}
}

// LocalAddr 返回本地地址
func (w *WSConn) LocalAddr() net.Addr {
	if w.localAddr != nil {
		return w.localAddr
	}
	return w.Conn.LocalAddr()
}

// RemoteAddr 返回远程地址
func (w *WSConn) RemoteAddr() net.Addr {
	if w.remoteAddr != nil {
		return w.remoteAddr
	}
	return w.Conn.RemoteAddr()
}

// Pool WebSocket连接池结构体
type Pool struct {
	conns     sync.Map                 // 连接存储
	idChan    chan string              // 连接ID通道
	clientIP  string                   // 客户端IP白名单
	tlsConfig *tls.Config              // TLS配置
	dialer    func() (net.Conn, error) // 连接拨号函数
	server    *http.Server             // HTTP服务器
	listener  net.Listener             // 网络监听器
	first     atomic.Bool              // 首次标志
	errCount  atomic.Int32             // 错误计数
	capacity  atomic.Int32             // 当前容量
	minCap    int                      // 最小容量
	maxCap    int                      // 最大容量
	interval  atomic.Int64             // 当前间隔
	minIvl    time.Duration            // 最小间隔
	maxIvl    time.Duration            // 最大间隔
	keepAlive time.Duration            // 保活时间
	ctx       context.Context          // 上下文
	cancel    context.CancelFunc       // 取消函数
}

// NewClientPool 创建客户端连接池
func NewClientPool(minCap, maxCap int, minIvl, maxIvl, keepAlive time.Duration, tlsCode, serverURL string) *Pool {
	if minCap <= 0 {
		minCap = defaultMinCap
	}
	if maxCap <= 0 {
		maxCap = defaultMaxCap
	}
	if minCap > maxCap {
		minCap, maxCap = maxCap, minCap
	}

	if minIvl <= 0 {
		minIvl = defaultMinIvl
	}
	if maxIvl <= 0 {
		maxIvl = defaultMaxIvl
	}
	if minIvl > maxIvl {
		minIvl, maxIvl = maxIvl, minIvl
	}

	var tlsConfig *tls.Config
	switch tlsCode {
	case "0", "1":
		// 使用自签名证书（不验证）
		tlsConfig = &tls.Config{
			InsecureSkipVerify: true,
			MinVersion:         tls.VersionTLS13,
		}
	default:
		// 使用验证证书（安全模式）
		tlsConfig = &tls.Config{
			InsecureSkipVerify: false,
			MinVersion:         tls.VersionTLS13,
		}
	}

	wsURL := "wss://" + serverURL + wsPath
	pool := &Pool{
		conns:     sync.Map{},
		idChan:    make(chan string, maxCap),
		tlsConfig: tlsConfig,
		minCap:    minCap,
		maxCap:    maxCap,
		minIvl:    minIvl,
		maxIvl:    maxIvl,
		keepAlive: keepAlive,
	}
	pool.dialer = func() (net.Conn, error) { return pool.dialWebSocket(wsURL) }
	pool.capacity.Store(int32(minCap))
	pool.interval.Store(int64(minIvl))
	pool.ctx, pool.cancel = context.WithCancel(context.Background())
	return pool
}

// NewServerPool 创建服务端连接池
func NewServerPool(maxCap int, clientIP string, tlsConfig *tls.Config, listener net.Listener, keepAlive time.Duration) *Pool {
	if maxCap <= 0 {
		maxCap = defaultMaxCap
	}

	if listener == nil || tlsConfig == nil {
		return nil
	}

	pool := &Pool{
		conns:     sync.Map{},
		idChan:    make(chan string, maxCap),
		clientIP:  clientIP,
		tlsConfig: tlsConfig,
		listener:  listener,
		maxCap:    maxCap,
		keepAlive: keepAlive,
	}
	pool.ctx, pool.cancel = context.WithCancel(context.Background())
	return pool
}

// dialWebSocket 创建WebSocket客户端连接
func (p *Pool) dialWebSocket(wsURL string) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(p.ctx, 30*time.Second)
	defer cancel()

	var tlsState *tls.ConnectionState
	var localAddr, remoteAddr net.Addr

	// 自定义HTTP客户端以捕获TLS状态和地址信息
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig:     p.tlsConfig,
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
			DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				conn, err := (&tls.Dialer{Config: p.tlsConfig}).DialContext(ctx, network, addr)
				if err != nil {
					return nil, err
				}
				if tlsConn, ok := conn.(*tls.Conn); ok {
					state := tlsConn.ConnectionState()
					tlsState = &state
					localAddr = tlsConn.LocalAddr()
					remoteAddr = tlsConn.RemoteAddr()
				}
				return conn, nil
			},
		},
	}

	wsConn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{
		HTTPClient:      httpClient,
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		return nil, fmt.Errorf("dialWebSocket: %w", err)
	}
	wsConn.SetReadLimit(-1)

	return &WSConn{
		Conn:       websocket.NetConn(p.ctx, wsConn, websocket.MessageBinary),
		tlsState:   tlsState,
		localAddr:  localAddr,
		remoteAddr: remoteAddr,
	}, nil
}

// createConnection 创建客户端连接并完成ID交换
func (p *Pool) createConnection() bool {
	conn, err := p.dialer()
	if err != nil {
		return false
	}

	conn.SetReadDeadline(time.Now().Add(idReadTimeout))
	buf := make([]byte, 4)
	n, err := io.ReadFull(conn, buf)
	if err != nil || n != 4 {
		conn.Close()
		return false
	}
	id := hex.EncodeToString(buf)
	conn.SetReadDeadline(time.Time{})

	p.conns.Store(id, conn)
	select {
	case p.idChan <- id:
		return true
	default:
		p.conns.Delete(id)
		conn.Close()
		return false
	}
}

// handleConnection 处理服务端WebSocket连接
func (p *Pool) handleConnection(w http.ResponseWriter, r *http.Request) {
	if p.Active() >= p.maxCap {
		http.Error(w, "pool full", http.StatusServiceUnavailable)
		return
	}

	clientIP := r.RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		clientIP = host
	}
	if p.clientIP != "" && clientIP != p.clientIP {
		http.Error(w, "unauthorized", http.StatusForbidden)
		return
	}

	wsConn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		CompressionMode:    websocket.CompressionDisabled,
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}
	wsConn.SetReadLimit(-1)

	rawID, id, err := p.generateID()
	if err != nil {
		wsConn.Close(websocket.StatusInternalError, "id generation failed")
		return
	}

	if _, exist := p.conns.Load(id); exist {
		wsConn.Close(websocket.StatusInternalError, "duplicate id")
		return
	}

	var tlsState *tls.ConnectionState
	var localAddr, remoteAddr net.Addr
	if r.TLS != nil {
		tlsState = r.TLS
	}
	if p.listener != nil {
		localAddr = p.listener.Addr()
	}
	if addr, err := net.ResolveTCPAddr("tcp", r.RemoteAddr); err == nil {
		remoteAddr = addr
	}

	wrappedConn := &WSConn{
		Conn:       websocket.NetConn(p.ctx, wsConn, websocket.MessageBinary),
		tlsState:   tlsState,
		localAddr:  localAddr,
		remoteAddr: remoteAddr,
	}
	if _, err := wrappedConn.Write(rawID); err != nil {
		wrappedConn.Close()
		return
	}

	select {
	case p.idChan <- id:
		p.conns.Store(id, wrappedConn)
	default:
		wrappedConn.Close()
	}
}

// ClientManager 客户端连接池管理器
func (p *Pool) ClientManager() {
	if p.cancel != nil {
		p.cancel()
	}
	p.ctx, p.cancel = context.WithCancel(context.Background())

	for p.ctx.Err() == nil {
		p.adjustInterval()
		capacity := int(p.capacity.Load())
		need := capacity - len(p.idChan)
		created := 0

		if need > 0 {
			var wg sync.WaitGroup
			results := make(chan int, need)
			for range need {
				wg.Go(func() {
					if p.createConnection() {
						results <- 1
					}
				})
			}
			wg.Wait()
			close(results)
			for r := range results {
				created += r
			}
		}

		p.adjustCapacity(created)

		select {
		case <-p.ctx.Done():
			return
		case <-time.After(time.Duration(p.interval.Load())):
		}
	}
}

// ServerManager 服务端连接池管理器
func (p *Pool) ServerManager() {
	if p.cancel != nil {
		p.cancel()
	}
	p.ctx, p.cancel = context.WithCancel(context.Background())

	tlsListener := tls.NewListener(p.listener, p.tlsConfig)
	mux := http.NewServeMux()
	mux.HandleFunc(wsPath, p.handleConnection)

	p.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       p.keepAlive,
		ErrorLog:          log.New(io.Discard, "", 0),
	}

	if err := p.server.Serve(tlsListener); err != nil && err != http.ErrServerClosed {
		if p.ctx.Err() == nil {
			p.errCount.Add(1)
		}
	}
}

// OutgoingGet 根据ID获取池连接
func (p *Pool) OutgoingGet(id string, timeout time.Duration) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(p.ctx, timeout)
	defer cancel()

	for {
		if conn, ok := p.conns.LoadAndDelete(id); ok {
			select {
			case <-p.idChan:
			default:
			}
			return conn.(net.Conn), nil
		}

		select {
		case <-time.After(idRetryInterval):
		case <-ctx.Done():
			return nil, fmt.Errorf("OutgoingGet: connection not found")
		}
	}
}

// IncomingGet 获取池连接并返回ID
func (p *Pool) IncomingGet(timeout time.Duration) (string, net.Conn, error) {
	ctx, cancel := context.WithTimeout(p.ctx, timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return "", nil, fmt.Errorf("IncomingGet: insufficient connections")
		case id := <-p.idChan:
			if conn, ok := p.conns.LoadAndDelete(id); ok {
				return id, conn.(net.Conn), nil
			}
		}
	}
}

// Flush 清空连接池
func (p *Pool) Flush() {
	var wg sync.WaitGroup
	p.conns.Range(func(key, value any) bool {
		wg.Go(func() {
			if value != nil {
				value.(net.Conn).Close()
			}
		})
		return true
	})
	wg.Wait()

	p.conns = sync.Map{}
	p.idChan = make(chan string, p.maxCap)
}

// Close 关闭连接池
func (p *Pool) Close() {
	if p.cancel != nil {
		p.cancel()
	}
	if p.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		p.server.Shutdown(ctx)
	}
	p.Flush()
}

// Ready 检查连接池是否就绪
func (p *Pool) Ready() bool {
	return p.ctx != nil
}

// Active 获取活跃连接数
func (p *Pool) Active() int {
	return len(p.idChan)
}

// Capacity 获取当前容量
func (p *Pool) Capacity() int {
	return int(p.capacity.Load())
}

// Interval 获取当前间隔
func (p *Pool) Interval() time.Duration {
	return time.Duration(p.interval.Load())
}

// AddError 增加错误计数
func (p *Pool) AddError() {
	p.errCount.Add(1)
}

// ErrorCount 获取错误计数
func (p *Pool) ErrorCount() int {
	return int(p.errCount.Load())
}

// ResetError 重置错误计数
func (p *Pool) ResetError() {
	p.errCount.Store(0)
}

// adjustInterval 动态调整连接创建间隔
func (p *Pool) adjustInterval() {
	idle := len(p.idChan)
	capacity := int(p.capacity.Load())
	interval := time.Duration(p.interval.Load())

	if idle < int(float64(capacity)*intervalLowThreshold) && interval > p.minIvl {
		newInterval := max(interval-intervalAdjustStep, p.minIvl)
		p.interval.Store(int64(newInterval))
	}

	if idle > int(float64(capacity)*intervalHighThreshold) && interval < p.maxIvl {
		newInterval := min(interval+intervalAdjustStep, p.maxIvl)
		p.interval.Store(int64(newInterval))
	}
}

// adjustCapacity 动态调整连接池容量
func (p *Pool) adjustCapacity(created int) {
	capacity := int(p.capacity.Load())
	ratio := float64(created) / float64(capacity)

	if ratio < capacityAdjustLowRatio && capacity > p.minCap {
		p.capacity.Add(-1)
	}

	if ratio > capacityAdjustHighRatio && capacity < p.maxCap {
		p.capacity.Add(1)
	}
}

// generateID 生成唯一连接ID
func (p *Pool) generateID() ([]byte, string, error) {
	if p.first.CompareAndSwap(false, true) {
		return []byte{0, 0, 0, 0}, "00000000", nil
	}

	rawID := make([]byte, 4)
	if _, err := rand.Read(rawID); err != nil {
		return nil, "", err
	}
	id := hex.EncodeToString(rawID)
	return rawID, id, nil
}
