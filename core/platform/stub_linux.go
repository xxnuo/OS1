//go:build linux

package platform

func NewPlatformAdapter() Adapter {
	return NewStub()
}
