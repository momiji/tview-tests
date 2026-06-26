package processor

import (
	"io"
	"sync"

	"test/internal/proxy/message"
)

// transportTunnel treats the established connection as a forever duplex pipe
// (axis C = tunnel), used for CONNECT and 100-Continue. It copies both
// directions concurrently and closes when either side finishes.
func (p *Process) transportTunnel(clientChannel, proxyChannel *message.ProxyRequest) *message.ProxyRequest {
	var finished sync.WaitGroup
	finished.Add(2)
	go p.pipe(clientChannel, proxyChannel, &finished)
	go p.pipe(proxyChannel, clientChannel, &finished)
	finished.Wait()
	return p.closeChannels(clientChannel, proxyChannel)
}

func (p *Process) pipe(source *message.ProxyRequest, target *message.ProxyRequest, wait *sync.WaitGroup) {
	// io.Copy uses splice/sendfile (zerocopy) only if src/dst are *net.TCPConn
	_, _ = io.Copy(target.Conn(), source.Conn())
	// in a forever pipe, just close connections after copy is finished
	p.closeChannel(source)
	p.closeChannel(target)
	wait.Done()
}
