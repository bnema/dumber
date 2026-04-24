package netguard

import (
	"net"
	"testing"
)

func TestIsBlockedTranscodeIPRejectsSpecialPurposeRanges(t *testing.T) {
	tests := []string{
		"127.0.0.1",
		"10.0.0.1",
		"172.16.0.1",
		"192.168.1.1",
		"100.64.0.1",
		"198.18.0.1",
		"192.0.2.1",
		"169.254.169.254",
		"0.0.0.0",
		"64:ff9b::a00:1",
		"2001::1",
		"2001:db8::1",
		"2002:0a00:0001::1",
		"fc00::1",
		"fe80::1",
		"::ffff:127.0.0.1",
	}
	for _, rawIP := range tests {
		t.Run(rawIP, func(t *testing.T) {
			if !IsBlockedTranscodeIP(net.ParseIP(rawIP)) {
				t.Fatalf("IsBlockedTranscodeIP(%s) = false, want true", rawIP)
			}
		})
	}
}

func TestIsBlockedTranscodeIPRejectsInvalidIP(t *testing.T) {
	if !IsBlockedTranscodeIP(net.ParseIP("")) {
		t.Fatal("IsBlockedTranscodeIP(invalid IP) = false, want true")
	}
}

func TestIsBlockedTranscodeIPAllowsPublicAddress(t *testing.T) {
	for _, rawIP := range []string{"93.184.216.34", "2606:4700:4700::1111"} {
		t.Run(rawIP, func(t *testing.T) {
			if IsBlockedTranscodeIP(net.ParseIP(rawIP)) {
				t.Fatalf("IsBlockedTranscodeIP(%s) = true, want false", rawIP)
			}
		})
	}
}
