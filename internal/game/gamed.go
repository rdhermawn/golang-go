package game

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"net"
	"strings"
	"time"
	"unicode/utf16"

	"golang-refine/internal/config"
)

const opcodeGetRoleBase = 3013

type Gamed struct {
	cfg     *config.Config
	cycle   int
	online  bool
	opcodes map[string]int
}

func NewGamed(cfg *config.Config) *Gamed {
	g := &Gamed{
		cfg:   cfg,
		cycle: 0,
	}
	g.online = g.ServerOnline()
	g.opcodes = map[string]int{
		"getRoleBase":            opcodeGetRoleBase,
		"getUserRoles":           3401,
		"renameRole":             3404,
		"getOnlineList":          352,
		"getRole":                8003,
		"putRole":                8002,
		"getUser":                3002,
		"getRoleStatus":          3015,
		"getRoleInventory":       3053,
		"getRoleEquipment":       3017,
		"getRoleStoreHouse":      3027,
		"getRoleTask":            3019,
		"sendMail":               4214,
		"worldChat":              120,
		"chat2player":            96,
		"forbidAcc":              5035,
		"forbidRole":             360,
		"muteAcc":                362,
		"muteRole":               356,
		"getRoleid":              3033,
		"getTerritory":           863,
		"getRoleFriend":          201,
		"AddFaction":             4600,
		"DelFaction":             4601,
		"getFactionInfo":         4606,
		"getUserExtraInfo":       384,
		"getUserFaction":         4607,
		"getFactionDetail":       4608,
		"FactionUpgrade":         4610,
		"GFactionFortressDetail": 4404,
		"Debug":                  873,
		"sendGold":               521,
	}
	return g
}

func (g *Gamed) ServerOnline() bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", g.cfg.IP, g.cfg.Ports.Gamedbd), g.timeout())
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func (g *Gamed) timeout() time.Duration {
	return 1 * time.Second
}

func (g *Gamed) DeleteHeader(data []byte) []byte {
	p := 0
	unpackCuint(data, &p)
	unpackCuint(data, &p)
	p += 8
	if p >= len(data) {
		return nil
	}
	return data[p:]
}

func (g *Gamed) CreateHeader(opcode int, data []byte) []byte {
	header := cuint(opcode)
	header = append(header, cuint(len(data))...)
	return append(header, data...)
}

func (g *Gamed) PackString(s string) []byte {
	runes := []rune(s)
	utf16le := make([]uint16, len(runes))
	for i, r := range runes {
		utf16le[i] = uint16(r)
	}
	buf := make([]byte, len(utf16le)*2)
	for i, v := range utf16le {
		buf[i*2] = byte(v)
		buf[i*2+1] = byte(v >> 8)
	}
	prefix := cuint(len(buf))
	return append(prefix, buf...)
}

func (g *Gamed) PackOctet(data string) []byte {
	b := hexDecode(data)
	prefix := cuint(len(b))
	return append(prefix, b...)
}

func (g *Gamed) PackInt(v int) []byte {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, uint32(v))
	return buf
}

func (g *Gamed) PackByte(v byte) []byte {
	return []byte{v}
}

func (g *Gamed) PackFloat(v float32) []byte {
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, math.Float32bits(v))
	return buf
}

func (g *Gamed) PackShort(v int) []byte {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, uint16(v))
	return buf
}

func cuint(data int) []byte {
	if data < 64 {
		return []byte{byte(data)}
	} else if data < 16384 {
		v := uint16(data) | 0x8000
		buf := make([]byte, 2)
		binary.BigEndian.PutUint16(buf, v)
		return buf
	} else if data < 536870912 {
		v := uint32(data) | 0xC0000000
		buf := make([]byte, 4)
		binary.BigEndian.PutUint32(buf, v)
		return buf
	}
	buf := make([]byte, 5)
	buf[0] = 0xE0
	binary.LittleEndian.PutUint32(buf[1:], uint32(data))
	return buf
}

func unpackCuint(data []byte, p *int) int {
	if *p >= len(data) {
		return 0
	}
	b := data[*p]
	var size int
	var min int
	if b < 0x80 {
		size = 1
	} else if b < 0xC0 {
		size = 2
		min = 0x8000
	} else if b < 0xE0 {
		size = 4
		min = 0xC0000000
	} else {
		*p++
		size = 4
	}
	if *p+size > len(data) {
		*p = len(data)
		return 0
	}
	var val int
	switch size {
	case 1:
		val = int(data[*p])
	case 2:
		val = int(binary.BigEndian.Uint16(data[*p:]))
	case 4:
		val = int(binary.BigEndian.Uint32(data[*p:]))
	}
	*p += size
	return val - min
}

func unpackLong(data []byte) (int64, bool) {
	if len(data) < 8 {
		return 0, false
	}
	high := binary.BigEndian.Uint32(data[0:4])
	low := binary.BigEndian.Uint32(data[4:8])
	return int64(high)<<32 | int64(low), true
}

func unpackOctet(data []byte, p *int) string {
	size := unpackCuint(data, p)
	if *p+size > len(data) {
		*p = len(data)
		return ""
	}
	octet := hex.EncodeToString(data[*p : *p+size])
	*p += size
	return octet
}

func unpackString(data []byte, p *int) string {
	if *p >= len(data) {
		return ""
	}
	h := data[*p]
	var sizeLen int
	var octetLen int
	if h >= 128 {
		sizeLen = 2
		v := int(binary.BigEndian.Uint16(data[*p:]))
		octetLen = v - 32768
	} else {
		sizeLen = 1
		octetLen = int(h)
	}
	pp := *p + sizeLen
	*p = pp + octetLen
	if *p > len(data) {
		*p = len(data)
		return ""
	}
	raw := data[pp:*p]
	u16s := make([]uint16, len(raw)/2)
	for i := 0; i < len(u16s); i++ {
		u16s[i] = uint16(raw[i*2]) | uint16(raw[i*2+1])<<8
	}
	runes := utf16.Decode(u16s)
	return string(runes)
}

func (g *Gamed) SendToGamedBD(data []byte) ([]byte, error) {
	return g.SendToSocket(data, g.cfg.Ports.Gamedbd, false)
}

func (g *Gamed) SendToDelivery(data []byte) ([]byte, error) {
	return g.SendToSocket(data, g.cfg.Ports.Gdeliveryd, true)
}

func (g *Gamed) SendToSocket(data []byte, port int, recvAfterSend bool) ([]byte, error) {
	addr := fmt.Sprintf("%s:%d", g.cfg.IP, port)
	conn, err := net.DialTimeout("tcp", addr, g.timeout())
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if recvAfterSend {
		conn.SetReadDeadline(time.Now().Add(g.timeout()))
		tmp := make([]byte, 8192)
		if _, err := conn.Read(tmp); err != nil {
			return nil, fmt.Errorf("recv after send read: %w", err)
		}
	}

	if _, err := conn.Write(data); err != nil {
		return nil, err
	}

	var buf []byte
	conn.SetReadDeadline(time.Now().Add(g.timeout()))

	switch g.cfg.SReadType {
	case 1:
		tmp := make([]byte, g.cfg.MaxBuffer)
		n, err := conn.Read(tmp)
		if err != nil {
			return nil, fmt.Errorf("sread type 1: %w", err)
		}
		buf = tmp[:n]
	case 2:
		tmp := make([]byte, 1024)
		for {
			conn.SetReadDeadline(time.Now().Add(g.timeout()))
			n, err := conn.Read(tmp)
			if err != nil {
				if len(buf) > 0 {
					break
				}
				return nil, fmt.Errorf("sread type 2: %w", err)
			}
			buf = append(buf, tmp[:n]...)
			if n < 1024 {
				break
			}
		}
	case 3:
		tmp := make([]byte, 1024)
		n, err := conn.Read(tmp)
		if err != nil {
			return nil, fmt.Errorf("sread type 3 initial read: %w", err)
		}
		buf = append(buf, tmp[:n]...)
		if len(buf) >= 8 {
			tp := 0
			unpackCuint(buf, &tp)
			length := unpackCuint(buf, &tp)
			for len(buf) < length {
				conn.SetReadDeadline(time.Now().Add(g.timeout()))
				n, err := conn.Read(tmp)
				if err != nil {
					break
				}
				buf = append(buf, tmp[:n]...)
			}
		}
	default:
		return nil, fmt.Errorf("unsupported s_readtype: %d", g.cfg.SReadType)
	}

	return buf, nil
}

func hexDecode(s string) []byte {
	s = strings.TrimSpace(s)
	if len(s)%2 != 0 {
		s = "0" + s
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil
	}
	return b
}
