package main

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

func inc(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

func ExpandCIDRs(netblocks []string) ([]string, error) {
	var hosts []string
	for _, netblock := range netblocks {
		ip, ipnet, err := net.ParseCIDR(netblock)
		if err != nil {
			return hosts, err
		}
		for h := ip.Mask(ipnet.Mask); ipnet.Contains(h); inc(h) {
			hosts = append(hosts, h.String())
		}
	}
	return hosts, nil
}

//EnumerateHosts expands a list of cidrs and a list of cidrs to exclude and returns
//a full list of ip addresses.
func EnumerateHosts(netblocks []string, exclude []string) ([]string, error) {
	var hosts []string
	allHosts, err := ExpandCIDRs(netblocks)
	if err != nil {
		return hosts, err
	}

	allExcludeHosts, err := ExpandCIDRs(exclude)
	if err != nil {
		return hosts, err
	}
	excludeHosts := make(map[string]bool)
	for _, ip := range allExcludeHosts {
		excludeHosts[ip] = true
	}

	for _, ip := range allHosts {
		if _, excluded := excludeHosts[ip]; !excluded {
			hosts = append(hosts, ip)
		}
	}
	return hosts, nil
}

//EnumeratePorts expands a list of port specifications
//ports can be comma seprated or include dashed ranges.
func EnumeratePorts(portspec string) ([]int, error) {
	var ports []int
	isComma := func(c rune) bool {
		return c == ','
	}
	isDash := func(c rune) bool {
		return c == '-'
	}
	parts := strings.FieldsFunc(portspec, isComma)
	for _, part := range parts {
		hasDash := strings.ContainsRune(part, '-')
		rangeParts := strings.FieldsFunc(part, isDash)
		if hasDash && len(rangeParts) == 1 {
			return ports, fmt.Errorf("Invalid port specification: %v", part)
		}
		switch len(rangeParts) {
		case 1:
			port, err := strconv.Atoi(rangeParts[0])
			if err != nil {
				return ports, err
			}
			ports = append(ports, port)
		case 2:
			start, err := strconv.Atoi(rangeParts[0])
			if err != nil {
				return ports, err
			}
			end, err := strconv.Atoi(rangeParts[1])
			if err != nil {
				return ports, err
			}
			for port := start; port < end+1; port++ {
				ports = append(ports, port)
			}
		}
	}

	return ports, nil
}
