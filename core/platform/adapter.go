package platform

type Adapter interface {
	SetHUDVisible(visible bool) error
	RegisterHotkeyF7(handler func()) error
	SetTrayMute(mute bool) error
	Notify(title, body string) error
}

type Stub struct {
	hudVisible bool
	mute       bool
}

func NewStub() *Stub {
	return &Stub{hudVisible: true}
}

func (s *Stub) SetHUDVisible(visible bool) error {
	s.hudVisible = visible
	return nil
}

func (s *Stub) RegisterHotkeyF7(handler func()) error {
	if handler != nil {
		handler()
	}
	return nil
}

func (s *Stub) SetTrayMute(mute bool) error {
	s.mute = mute
	return nil
}

func (s *Stub) Notify(title, body string) error {
	return nil
}
