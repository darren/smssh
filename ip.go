package main

import (
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
)

const (
	redirectMode1  = 0x01
	redirectMode2  = 0x02
	ipRecordLength = 7
	maxCacheSize   = 100000
)

type Seeker struct {
	buffer []byte
	start  int64
	end    int64
	count  int64
}

type Location struct {
	Country string
	Area    string
}

func (loc *Location) String() string {
	return fmt.Sprintf("%s-%s", loc.Country, loc.Area)
}

var NullLocation = Location{"", ""}

// NewSeeker 默认读取 ~/.config/smssh/ 下的文件
func NewSeeker(filename string) (s *Seeker, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	fullname := path.Join(home, ".config", "smssh", filename)
	file, err := os.Open(fullname)

	if err != nil {
		return nil, err
	}

	buf, err := ioutil.ReadAll(file)

	if err != nil {
		return nil, err
	}

	indexStart := int64(binary.LittleEndian.Uint32(buf[0:4]))
	indexEnd := int64(binary.LittleEndian.Uint32(buf[4:8]))
	count := (indexEnd - indexStart) / ipRecordLength

	return &Seeker{buf, indexStart, indexEnd, count}, nil
}

func getMiddleOffset(begin, end int64) int64 {
	records := (end - begin) / ipRecordLength
	records >>= 1
	if records == 0 {
		records = 1
	}
	return begin + records*ipRecordLength
}

func (s *Seeker) readIP(offset int64) uint32 {
	buf := s.buffer[offset : offset+4]
	return binary.LittleEndian.Uint32(buf)
}

func (s *Seeker) Lookup(ipstr string) *Location {
	ipstr = strings.ReplaceAll(ipstr, "*", "1")
	var ipv4 net.IP

	if strings.Contains(ipstr, ".") {
		ipv4 = net.ParseIP(ipstr).To4()
	} else {
		if i, err := strconv.ParseInt(ipstr, 10, 64); err == nil {
			buf := make([]byte, 4)
			binary.BigEndian.PutUint32(buf, uint32(i))
			ipv4 = buf
		}
	}

	if ipv4 == nil {
		return &NullLocation
	}

	var loc Location

	ip := binary.BigEndian.Uint32(ipv4)
	var m int64

	for i, j := s.start, s.end; i < j; {
		m = getMiddleOffset(i, j)
		currentIP := s.readIP(m)

		if ip > currentIP {
			i = m
		} else if ip < currentIP {
			if j == m {
				j -= ipRecordLength
				m = j
			} else {
				j = m
			}
		} else {
			break
		}
	}

	m = readLong(s.buffer, m+4)
	endIP := s.readIP(m)

	if endIP < ip {
		loc = NullLocation
	} else {
		loc = getLocation(s.buffer, m)
	}

	return &loc
}

func getLocation(buffer []byte, offset int64) Location {
	var location Location
	offset += 4
	mode := buffer[offset]

	countryIdx := int64(0)
	areaIdx := int64(0)
	var countryLength int64

	if mode == redirectMode1 {
		countryIdx = readLong(buffer, offset+1)
		mode2 := buffer[countryIdx]

		if mode2 == redirectMode2 {
			location.Country, _ = readString(buffer, readLong(buffer, countryIdx+1))
			areaIdx = countryIdx + 4
		} else {
			location.Country, countryLength = readString(buffer, countryIdx)
			areaIdx = countryIdx + countryLength + 1
		}
	} else if mode == redirectMode2 {

		location.Country, _ = readString(buffer, readLong(buffer, offset+1))
		areaIdx = offset + 4
	} else {

		location.Country, countryLength = readString(buffer, offset)
		areaIdx = offset + countryLength + 1
	}
	location.Area = readArea(buffer, areaIdx)
	return location
}

func readArea(buffer []byte, offset int64) (s string) {
	mode := buffer[offset]

	if mode == redirectMode1 || mode == redirectMode2 {
		areaIdx := readLong(buffer, offset+1)
		if areaIdx == 0 {
			s = ""
			return
		}
		s, _ = readString(buffer, areaIdx)
		return
	}

	s, _ = readString(buffer, offset)
	return
}

func readLong(buffer []byte, offset int64) int64 {
	buf := make([]byte, 4)
	copy(buf, buffer[offset:offset+4])
	buf[3] = 0x00
	return int64(binary.LittleEndian.Uint32(buf))
}

func readString(buffer []byte, offset int64) (string, int64) {
	var i int64
	for i = 0; i < 1024; i++ {
		if buffer[offset+i] == 0x00 {
			break
		}
	}

	buf := make([]byte, i)
	copy(buf, buffer[offset:offset+i])

	return string(buf[0:i]), i
}

var ipRegex = regexp.MustCompile(`([0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9*]{1,3})`)

func (s *Seeker) ReplaceIP(buf []byte) []byte {
	return []byte(s.ReplaceIPString(string(buf)))
}

func (s *Seeker) ReplaceIPString(src string) string {
	matches := ipRegex.FindAllStringSubmatch(src, -1)
	ips := make(map[string]bool)
	if len(matches) > 0 {
		for _, m := range matches {
			ips[m[0]] = true
		}
	}

	for ip := range ips {
		loc := s.Lookup(ip)
		if loc != &NullLocation {
			src = strings.ReplaceAll(src, ip, loc.String())
		}

	}

	return src
}
