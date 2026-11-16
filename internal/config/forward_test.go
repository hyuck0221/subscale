package config

import "testing"

func TestForwardEndpoints(t *testing.T) {
	for input, want := range map[string]string{
		"DB.Example.:5432":       "db.example:5432",
		"100.89.207.112:5900":    "100.89.207.112:5900",
		"[fd7a:115c:a1e0::1]:22": "[fd7a:115c:a1e0::1]:22",
	} {
		got, err := Endpoint(input)
		if err != nil || got != want {
			t.Errorf("Endpoint(%q) = %q, %v", input, got, err)
		}
	}
	for _, input := range []string{"host", "host:0", "host:65536", "host:+22", ":22", "*.mds:5900", "http://host:80", "host:abc", "::1:22"} {
		if _, err := Endpoint(input); err == nil {
			t.Errorf("accepted %q", input)
		}
	}
}

func TestPersistedForwardsValidation(t *testing.T) {
	base := Forward{Account: "work", Target: "db.example:5432", LocalPort: 15432}
	for _, tc := range []struct {
		name     string
		forwards []Forward
		valid    bool
	}{
		{"legacy", nil, true},
		{"valid", []Forward{base}, true},
		{"unknown account", []Forward{{Account: "missing", Target: base.Target, LocalPort: base.LocalPort}}, false},
		{"port zero", []Forward{{Account: "work", Target: base.Target}}, false},
		{"proxy port", []Forward{{Account: "work", Target: base.Target, LocalPort: 18765}}, false},
		{"invalid target", []Forward{{Account: "work", Target: "*.mds:22", LocalPort: 12345}}, false},
		{"duplicate port", []Forward{base, {Account: "work", Target: "other:22", LocalPort: base.LocalPort}}, false},
		{"duplicate target", []Forward{base, {Account: "work", Target: base.Target, LocalPort: 12345}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			c := Config{Accounts: map[string]Account{"work": {}}, Forwards: tc.forwards}
			if err := Save(dir, c); err != nil {
				t.Fatal(err)
			}
			loaded, err := Load(dir)
			if (err == nil) != tc.valid {
				t.Fatalf("Load: %v", err)
			}
			if tc.valid && len(loaded.Forwards) != len(tc.forwards) {
				t.Fatal("forwards not preserved")
			}
		})
	}
}
