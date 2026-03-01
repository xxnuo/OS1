//go:build windows

package platform

func NewPlatformAdapter() Adapter {
	return NewStub()
}
