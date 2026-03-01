//go:build darwin

package platform

func NewPlatformAdapter() Adapter {
	return NewStub()
}
