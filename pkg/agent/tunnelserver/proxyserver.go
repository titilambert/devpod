package tunnelserver

import (
	"bufio"
	"context"
	"encoding/json"
	"io"

	"github.com/loft-sh/devpod/pkg/agent/tunnel"
	"github.com/loft-sh/devpod/pkg/devcontainer/config"
	"github.com/loft-sh/devpod/pkg/stdio"
	"github.com/loft-sh/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func RunProxyServer(ctx context.Context, client tunnel.TunnelClient, reader io.Reader, writer io.WriteCloser, log log.Logger) (*config.Result, error) {
	lis := stdio.NewStdioListener(reader, writer, false)
	s := grpc.NewServer()
	tunnelServ := &proxyServer{
		client: client,
		log:    log,
	}
	tunnel.RegisterTunnelServer(s, tunnelServ)
	reflection.Register(s)
	errChan := make(chan error, 1)
	go func() {
		errChan <- s.Serve(lis)
	}()

	select {
	case err := <-errChan:
		return nil, err
	case <-ctx.Done():
		return tunnelServ.result, nil
	}
}

type proxyServer struct {
	tunnel.UnimplementedTunnelServer

	client tunnel.TunnelClient
	result *config.Result
	log    log.Logger
}

func (t *proxyServer) ForwardPort(ctx context.Context, portRequest *tunnel.ForwardPortRequest) (*tunnel.ForwardPortResponse, error) {
	return t.client.ForwardPort(ctx, portRequest)
}

func (t *proxyServer) StopForwardPort(ctx context.Context, portRequest *tunnel.StopForwardPortRequest) (*tunnel.StopForwardPortResponse, error) {
	return t.client.StopForwardPort(ctx, portRequest)
}

func (t *proxyServer) DockerCredentials(ctx context.Context, message *tunnel.Message) (*tunnel.Message, error) {
	return t.client.DockerCredentials(ctx, message)
}

func (t *proxyServer) GitUser(ctx context.Context, empty *tunnel.Empty) (*tunnel.Message, error) {
	return t.client.GitUser(ctx, empty)
}

func (t *proxyServer) GitCredentials(ctx context.Context, message *tunnel.Message) (*tunnel.Message, error) {
	return t.client.GitCredentials(ctx, message)
}

func (t *proxyServer) SendResult(ctx context.Context, result *tunnel.Message) (*tunnel.Empty, error) {
	parsedResult := &config.Result{}
	err := json.Unmarshal([]byte(result.Message), parsedResult)
	if err != nil {
		return nil, err
	}

	t.result = parsedResult
	return t.client.SendResult(ctx, result)
}

func (t *proxyServer) Ping(ctx context.Context, message *tunnel.Empty) (*tunnel.Empty, error) {
	return t.client.Ping(ctx, message)
}

func (t *proxyServer) Log(ctx context.Context, message *tunnel.LogMessage) (*tunnel.Empty, error) {
	return t.client.Log(ctx, message)
}

func (t *proxyServer) GitCloneAndRead(response *tunnel.Empty, stream tunnel.Tunnel_GitCloneAndReadServer) error {
	t.log.Debug("Cloning and reading workspace")
	client, err := t.client.GitCloneAndRead(context.TODO(), &tunnel.Empty{})
	if err != nil {
		return err
	}

	buf := bufio.NewWriterSize(NewStreamWriter(stream, t.log), 10*1024)
	_, err = io.Copy(buf, NewStreamReader(client, t.log))
	if err != nil {
		return err
	}

	// make sure buffer is flushed
	return buf.Flush()
}

func (t *proxyServer) ReadWorkspace(response *tunnel.Empty, stream tunnel.Tunnel_ReadWorkspaceServer) error {
	t.log.Debug("Start reading workspace")

	client, err := t.client.ReadWorkspace(context.TODO(), &tunnel.Empty{})
	if err != nil {
		return err
	}

	buf := bufio.NewWriterSize(NewStreamWriter(stream, t.log), 10*1024)
	_, err = io.Copy(buf, NewStreamReader(client, t.log))
	if err != nil {
		return err
	}

	// make sure buffer is flushed
	return buf.Flush()
}
