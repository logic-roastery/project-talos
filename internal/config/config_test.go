package config

import "testing"

func TestProxyModeDefault(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		fallback ProxyMode
		want     ProxyMode
	}{
		{name: "internal", value: "internal", fallback: ProxyModeExternal, want: ProxyModeInternal},
		{name: "external", value: "external", fallback: ProxyModeInternal, want: ProxyModeExternal},
		{name: "invalid", value: "bogus", fallback: ProxyModeInternal, want: ProxyModeInternal},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("TALOS_PROXY_MODE", tc.value)
			if got := proxyModeDefault("TALOS_PROXY_MODE", tc.fallback); got != tc.want {
				t.Fatalf("proxyModeDefault() = %q, want %q", got, tc.want)
			}
		})
	}
}
