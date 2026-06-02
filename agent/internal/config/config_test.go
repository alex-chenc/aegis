package config

import "testing"

func TestDeriveAPIServerAddr(t *testing.T) {
	tests := []struct {
		name       string
		serverAddr string
		want       string
	}{
		{
			name:       "ipv4 host",
			serverAddr: "192.168.152.159:19090",
			want:       "http://192.168.152.159:8082",
		},
		{
			name:       "hostname",
			serverAddr: "aegis.example.com:19090",
			want:       "http://aegis.example.com:8082",
		},
		{
			name:       "empty",
			serverAddr: "",
			want:       "http://127.0.0.1:8082",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DeriveAPIServerAddr(tt.serverAddr); got != tt.want {
				t.Fatalf("DeriveAPIServerAddr(%q) = %q, want %q", tt.serverAddr, got, tt.want)
			}
		})
	}
}
