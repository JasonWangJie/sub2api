package service

import (
	"context"
	"sync"

	openaiwsv2 "github.com/Wei-Shaw/sub2api/internal/service/openai_ws_v2"
	coderws "github.com/coder/websocket"
	"github.com/tidwall/gjson"
)

type openAI429FrameConn struct {
	inner          openaiwsv2.FrameConn
	service        *OpenAI429ModeService
	account        *Account
	mu             sync.Mutex
	model          string
	ticket         *openAI429Ticket
	observer       openAI429Observer
	completed      map[string]struct{}
	completedOrder []string
}

func (c *openAI429FrameConn) WriteFrame(ctx context.Context, typ coderws.MessageType, payload []byte) error {
	c.mu.Lock()
	event := gjson.GetBytes(payload, "type").String()
	model := gjson.GetBytes(payload, "model").String()
	if event == "session.update" {
		if updated := gjson.GetBytes(payload, "session.model").String(); updated != "" {
			c.model = updated
		}
	}
	if model == "" {
		model = c.model
	}
	if event == "response.create" && openAI429TextRequest("/responses", model, payload) {
		if c.ticket != nil {
			c.mu.Unlock()
			return errOpenAI429Admission
		}
		ticket, _, err := c.service.beginFresh(ctx, c.account, model)
		if err != nil {
			c.mu.Unlock()
			return err
		}
		c.ticket = ticket
		c.observer = openAI429Observer{status: 200, stream: true}
		c.model = model
	}
	c.mu.Unlock()
	err := c.inner.WriteFrame(ctx, typ, payload)
	if err != nil {
		c.finishInterrupted()
	}
	return err
}

func (c *openAI429FrameConn) ReadFrame(ctx context.Context) (coderws.MessageType, []byte, error) {
	typ, payload, err := c.inner.ReadFrame(ctx)
	c.mu.Lock()
	if c.ticket != nil {
		responseID := gjson.GetBytes(payload, "response.id").String()
		if responseID == "" {
			responseID = gjson.GetBytes(payload, "response_id").String()
		}
		if _, duplicate := c.completed[responseID]; responseID != "" && duplicate && err == nil {
			c.mu.Unlock()
			return typ, payload, err
		}
		if err == nil {
			c.observer.document(payload)
		}
		if err != nil || c.observer.terminal {
			if responseID != "" && c.observer.terminal {
				if c.completed == nil {
					c.completed = make(map[string]struct{})
				}
				c.completed[responseID] = struct{}{}
				c.completedOrder = append(c.completedOrder, responseID)
				if len(c.completedOrder) > 256 {
					delete(c.completed, c.completedOrder[0])
					c.completedOrder = c.completedOrder[1:]
				}
			}
			ticket := c.ticket
			c.ticket = nil
			result := c.observer.result(false)
			c.mu.Unlock()
			ticket.finish(result)
			return typ, payload, err
		}
	}
	c.mu.Unlock()
	return typ, payload, err
}

func (c *openAI429FrameConn) finishInterrupted() {
	c.mu.Lock()
	ticket := c.ticket
	c.ticket = nil
	c.mu.Unlock()
	ticket.finish(openAI429Outcome{})
}

func (c *openAI429FrameConn) Close() error { c.finishInterrupted(); return c.inner.Close() }
