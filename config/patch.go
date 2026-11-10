package config

import "sync/atomic"

var (
	// The list is replaced whole rather than edited in place, so it rides one
	// pointer a reader takes without the lock: a host reads it on every answer
	// it gives, and a write lands once per config parse.
	proxyNameList atomic.Pointer[[]string]
)

func GetProxyNameList() []string {
	if list := proxyNameList.Load(); list != nil {
		return *list
	}
	return nil
}

func SetProxyNameList(list []string) {
	proxyNameList.Store(&list)
}
