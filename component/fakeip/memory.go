package fakeip

import (
	"net/netip"
	"sync"
)

type fakeipEntry struct {
	prev, next *fakeipEntry
	host       string
	ip         netip.Addr
}

type fakeipLRU struct {
	front, back *fakeipEntry
	len         int
}

func (l *fakeipLRU) pushBack(entry *fakeipEntry) {
	entry.prev = l.back
	entry.next = nil
	if l.back != nil {
		l.back.next = entry
	} else {
		l.front = entry
	}
	l.back = entry
	l.len++
}

func (l *fakeipLRU) remove(entry *fakeipEntry) {
	if entry.prev != nil {
		entry.prev.next = entry.next
	} else {
		l.front = entry.next
	}
	if entry.next != nil {
		entry.next.prev = entry.prev
	} else {
		l.back = entry.prev
	}
	entry.prev, entry.next = nil, nil
	l.len--
}

func (l *fakeipLRU) moveToBack(entry *fakeipEntry) {
	if l.back == entry {
		return
	}
	l.remove(entry)
	l.pushBack(entry)
}

func (l *fakeipLRU) init() {
	l.front, l.back, l.len = nil, nil, 0
}

type memoryStore struct {
	mu    sync.Mutex
	hosts map[string]*fakeipEntry
	ips   map[netip.Addr]*fakeipEntry
	lru   fakeipLRU
	size  int
}

func newMemoryStore(size int) *memoryStore {
	return &memoryStore{
		hosts: make(map[string]*fakeipEntry),
		ips:   make(map[netip.Addr]*fakeipEntry),
		size:  size,
	}
}

func (m *memoryStore) touch(entry *fakeipEntry) {
	m.lru.moveToBack(entry)
}

func (m *memoryStore) remove(entry *fakeipEntry) {
	delete(m.hosts, entry.host)
	delete(m.ips, entry.ip)
	m.lru.remove(entry)
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
	m.lru.pushBack(entry)

	if m.size > 0 && m.lru.len > m.size {
		if oldest := m.lru.front; oldest != nil {
			m.remove(oldest)
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

// ReplaceByIP implements store.ReplaceByIP
func (m *memoryStore) ReplaceByIP(ip netip.Addr, host string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.put(host, ip)
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
	target.lru.init()
	for source := m.lru.front; source != nil; source = source.next {
		entry := *source
		target.hosts[entry.host] = &entry
		target.ips[entry.ip] = &entry
		target.lru.pushBack(&entry)
	}
}

// FlushFakeIP implements store.FlushFakeIP
func (m *memoryStore) FlushFakeIP() error {
	m.mu.Lock()
	m.hosts = make(map[string]*fakeipEntry)
	m.ips = make(map[netip.Addr]*fakeipEntry)
	m.lru.init()
	m.mu.Unlock()
	return nil
}
