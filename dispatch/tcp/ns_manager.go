package tcp

import "sync"

type NSMap struct {
	mu   sync.RWMutex
	data map[string][]*Client
}

func (m *NSMap) Add(key string, value *Client) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.data == nil {
		m.data = make(map[string][]*Client)
	}

	m.data[key] = append(m.data[key], value)
}

func (m *NSMap) Get(key string) []*Client {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.data == nil {
		return nil
	}
	obj := m.data[key]

	return append([]*Client{}, obj...)
}

func (m *NSMap) Delete(key string, valueToDelete *Client) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil {
		return
	}

	slice := m.data[key]
	for i, v := range slice {
		if v == valueToDelete {
			obj := append(slice[:i], slice[i+1:]...)
			m.data[key] = *&obj
			break
		}
	}

	if len(m.data[key]) == 0 {
		delete(m.data, key)
	}
}
