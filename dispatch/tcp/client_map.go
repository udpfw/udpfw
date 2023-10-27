package tcp

import "sync"

type ClientMap struct {
	list sync.Map
}

func (c *ClientMap) RegisterClient(id string, cli *Client) {
	c.list.Store(id, cli)
}

func (c *ClientMap) Len() int {
	r := 0
	c.list.Range(func(_, _ any) bool {
		r += 1
		return true
	})
	return r
}

func (c *ClientMap) Delete(id string) {
	c.list.Delete(id)
}

func (c *ClientMap) Range(fn func(id string, cli *Client) bool) {
	c.list.Range(func(key, value any) bool {
		return fn(key.(string), value.(*Client))
	})
}
