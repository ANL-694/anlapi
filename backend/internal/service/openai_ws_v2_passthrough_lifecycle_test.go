package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

type passthroughLifecycleFrame struct {
	messageType coderws.MessageType
	payload     []byte
}

type passthroughLifecycleFrameConn struct {
	frames chan passthroughLifecycleFrame
	once   sync.Once
	closed chan struct{}
}

func newPassthroughLifecycleFrameConn() *passthroughLifecycleFrameConn {
	return &passthroughLifecycleFrameConn{
		frames: make(chan passthroughLifecycleFrame, 4),
		closed: make(chan struct{}),
	}
}

func (c *passthroughLifecycleFrameConn) ReadFrame(ctx context.Context) (coderws.MessageType, []byte, error) {
	select {
	case frame := <-c.frames:
		return frame.messageType, frame.payload, nil
	case <-c.closed:
		return coderws.MessageText, nil, errOpenAIWSConnClosed
	case <-ctx.Done():
		return coderws.MessageText, nil, ctx.Err()
	}
}

func (c *passthroughLifecycleFrameConn) WriteFrame(context.Context, coderws.MessageType, []byte) error {
	return nil
}

func (c *passthroughLifecycleFrameConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return nil
}

func TestOpenAIWSPassthroughTurnLifecycle_SerializesTerminalCommitAndNextTurn(t *testing.T) {
	clientFrameConn := &openAIWSClientFrameConn{interTurnStarted: make(chan struct{}, 1)}
	lifecycle := newOpenAIWSPassthroughTurnLifecycle(true)
	lifecycle.beginTerminalWrite()

	admitted := make(chan bool, 1)
	go func() {
		admitted <- lifecycle.beginResponseCreate(clientFrameConn.markTurnStarted)
	}()
	select {
	case <-admitted:
		t.Fatal("next response.create was admitted before terminal commit")
	case <-time.After(30 * time.Millisecond):
	}

	lifecycle.finishTerminalWrite(true, clientFrameConn.markTurnCompleted)
	select {
	case ok := <-admitted:
		require.True(t, ok)
	case <-time.After(time.Second):
		t.Fatal("next response.create remained blocked after terminal commit")
	}
	require.False(t, clientFrameConn.waitingForNextTurn.Load())
}

func TestOpenAIWSPassthroughFirstOutputFrameConn_TimesOutBeforeSemanticOutput(t *testing.T) {
	inner := newPassthroughLifecycleFrameConn()
	conn := &openAIWSPassthroughFirstOutputFrameConn{
		inner:           inner,
		deadlineChanged: make(chan struct{}, 1),
		resolveDeadline: func([]byte) openAIWSPassthroughFirstOutputDeadline {
			return openAIWSPassthroughFirstOutputDeadline{timeout: 30 * time.Millisecond, startedAt: time.Now()}
		},
	}
	require.NoError(t, conn.WriteFrame(context.Background(), coderws.MessageText, []byte(`{"type":"response.create"}`)))

	_, _, err := conn.ReadFrame(context.Background())
	require.Error(t, err)
	require.ErrorIs(t, err, errOpenAIWSPassthroughFirstOutputTimeout)
}

func TestOpenAIWSPassthroughFirstOutputFrameConn_ActiveReadTimeout(t *testing.T) {
	inner := newPassthroughLifecycleFrameConn()
	inner.frames <- passthroughLifecycleFrame{
		messageType: coderws.MessageText,
		payload:     []byte(`{"type":"response.output_text.delta","delta":"hello"}`),
	}
	conn := &openAIWSPassthroughFirstOutputFrameConn{
		inner:             inner,
		activeReadTimeout: 30 * time.Millisecond,
		deadlineChanged:   make(chan struct{}, 1),
		resolveDeadline: func([]byte) openAIWSPassthroughFirstOutputDeadline {
			return openAIWSPassthroughFirstOutputDeadline{timeout: time.Second, startedAt: time.Now()}
		},
	}
	require.NoError(t, conn.WriteFrame(context.Background(), coderws.MessageText, []byte(`{"type":"response.create"}`)))
	_, _, err := conn.ReadFrame(context.Background())
	require.NoError(t, err)

	_, _, err = conn.ReadFrame(context.Background())
	require.Error(t, err)
	require.True(t, errors.Is(err, errOpenAIWSPassthroughActiveTurnTimeout))
}
