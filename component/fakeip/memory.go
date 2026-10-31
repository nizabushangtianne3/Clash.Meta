package fakeip

import (
	"net/netip"
	"sync"

	list "github.com/bahlo/generic-list-go"
)

type fakeipEntry struct {
	element *list.Element[*fakeipEntry]
	host    string
	ip      netip.Addr
}

type memoryStore struct {
	mu    sync.Mutex
	hosts map[string]*fakeipEntry
	ips   map[netip.Addr]*fakeipEntry
	lru   *list.List[*fakeipEntry]
	size  int
}

func newMemoryStore(size int) *memoryStore {
	return &memoryStore{
		hosts: make(map[string]*fakeipEntry),
		ips:   make(map[netip.Addr]*fakeipEntry),
		lru:   list.New[*fakeipEntry](),
		size:  size,
	}
}

func (m *memoryStore) touch(entry *fakeipEntry) {
	m.lru.MoveToBack(entry.element)
}

func (m *memoryStore) remove(entry *fakeipEntry) {
	delete(m.hosts, entry.host)
	delete(m.ips, entry.ip)
	m.lru.Remove(entry.element)
}

func (m *memoryStore) put(host string, ip netip.Addr) {
	if entry := m.hosts[host]; entry != nil {
		if entry.ip != ip {
			delete(m.ips, entry.ip)
		}
		entry.ip = ip
		m.hosts[host] = entry
		m.ips[ip] = entry
		m.touch(entry)
		return
	}

	if entry := m.ips[ip]; entry != nil {
		delete(m.hosts, entry.host)
		entry.host = host
		m.hosts[host] = entry
		m.ips[ip] = entry
		m.touch(entry)
		return
	}

	entry := &fakeipEntry{host: host, ip: ip}
	m.hosts[host] = entry
	m.ips[ip] = entry
	entry.element = m.lru.PushBack(entry)

	if m.size > 0 && m.lru.Len() > m.size {
		if oldest := m.lru.Front(); oldest != nil {
			m.remove(oldest.Value)
		}
	}
}

// GetByHost implements store.GetByHost
func (m *memoryStore) GetByHost(host string) (netip.Addr, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry := m.hosts[host]
	if entry == nil {
		return netip.Addr{}, false
	}
	m.touch(entry)
	return entry.ip, true
}

// PutByHost implements store.PutByHost
func (m *memoryStore) PutByHost(host string, ip netip.Addr) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.put(host, ip)
}

// GetByIP implements store.GetByIP
func (m *memoryStore) GetByIP(ip netip.Addr) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry := m.ips[ip]
	if entry == nil {
		return "", false
	}
	m.touch(entry)
	return entry.host, true
}

// PutByIP implements store.PutByIP
func (m *memoryStore) PutByIP(ip netip.Addr, host string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.put(host, ip)
}

// DelByIP implements store.DelByIP
func (m *memoryStore) DelByIP(ip netip.Addr) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if entry := m.ips[ip]; entry != nil {
		m.remove(entry)
	}
}

// Exist implements store.Exist
func (m *memoryStore) Exist(ip netip.Addr) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ips[ip] != nil
}

// CloneTo implements store.CloneTo
// only for memoryStore to memoryStore
func (m *memoryStore) CloneTo(store store) {
	target, ok := store.(*memoryStore)
	if !ok {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	target.mu.Lock()
	defer target.mu.Unlock()

	target.hosts = make(map[string]*fakeipEntry)
	target.ips = make(map[netip.Addr]*fakeipEntry)
	target.lru.Init()
	for element := m.lru.Front(); element != nil; element = element.Next() {
		entry := *element.Value
		target.hosts[entry.host] = &entry
		target.ips[entry.ip] = &entry
		target.lru.PushBack(&entry)
	}
}

// FlushFakeIP implements store.FlushFakeIP
func (m *memoryStore) FlushFakeIP() error {
	m.mu.Lock()
	m.hosts = make(map[string]*fakeipEntry)
	m.ips = make(map[netip.Addr]*fakeipEntry)
	m.lru.Init()
	m.mu.Unlock()
	return nil
}
