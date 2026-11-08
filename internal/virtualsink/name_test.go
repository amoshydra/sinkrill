package virtualsink

import "testing"

func TestValidateName(t *testing.T) {
	valid := []string{"sonos", "sonos-test", "My_Sink.1"}
	for _, name := range valid {
		if err := validateName(name); err != nil {
			t.Errorf("validateName(%q) = %v, want nil", name, err)
		}
	}

	invalid := []string{"", "my sink", "sink/one", "sink:one", "sink\none"}
	for _, name := range invalid {
		if err := validateName(name); err == nil {
			t.Errorf("validateName(%q) = nil, want error", name)
		}
	}
}
