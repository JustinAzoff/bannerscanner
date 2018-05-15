package main

import "testing"

var testCases = []struct {
	include   []string
	exclude   []string
	expected  int
	wanterror bool
}{
	{[]string{"192.168.1.0/24"}, []string{}, 256, false},
	{[]string{"192.168.1.0/24"}, []string{"192.168.1.30/32"}, 255, false},
	{[]string{"192.168.1.0/24"}, []string{"192.168.1.30/30"}, 252, false},
	{[]string{"192.168.1.0/33"}, []string{}, 0, true},
}

func TestEnumerateHosts(t *testing.T) {
	for _, tt := range testCases {
		hosts, err := EnumerateHosts(tt.include, tt.exclude)
		if err != nil && tt.wanterror != true {
			t.Error(err)
		}
		if err == nil && tt.wanterror == true {
			t.Errorf("EnumerateHosts(%#v, %#v) did not return an error", tt.include, tt.exclude)
		}
		if len(hosts) != tt.expected {
			t.Errorf("EnumerateHosts(%#v, %#v) => len(hosts) is %#v, want %#v", tt.include, tt.exclude, len(hosts), tt.expected)
		}
	}
}

var portTestCases = []struct {
	port      string
	expected  []int
	wanterror bool
}{
	{"22", []int{22}, false},
	{"22,80,443", []int{22, 80, 443}, false},
	{"1-5,443", []int{1, 2, 3, 4, 5, 443}, false},
	{"1-,443", []int{}, true},
}

func TestEnumeratePorts(t *testing.T) {
	for _, tt := range portTestCases {
		ports, err := EnumeratePorts(tt.port)
		if err != nil && tt.wanterror != true {
			t.Error(err)
		}
		if err == nil && tt.wanterror == true {
			t.Errorf("EnumeratePorts(%#v) did not return an error", tt.port)
		}
		if len(ports) != len(tt.expected) {
			t.Errorf("EnumeratePorts(%#v) => len(ports) is %#v, want %v (%#v)", tt.port, len(ports), len(tt.expected), tt.expected)
			continue
		}
		for i, p := range ports {
			if p != tt.expected[i] {
				t.Errorf("EnumeratePorts(%#v) => ports[%d] wrong: got %v, want %v", tt.port, i, p, tt.expected[i])
			}
		}
	}
}
