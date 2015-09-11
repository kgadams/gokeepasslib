package gokeepasslib

import "encoding/base64"

var iv = []byte{0xe8, 0x30, 0x09, 0x4b, 0x97, 0x20, 0x5d, 0x2a}
var sigmaWords = []uint32{
	0x61707865,
	0x3320646e,
	0x79622d32,
	0x6b206574,
}

// SalsaManager is responsible for stream encrypting and decrypting of the passwords
type SalsaManager struct {
	State        []uint32
	blockUsed    int
	block        []byte
	counterWords [2]int
	currentBlock []byte
}

func (s *SalsaManager) UnlockGroups(gs []Group) {
	for i, _ := range gs { //For each top level group
		s.UnlockGroup(&gs[i])
	}
}
func (s *SalsaManager) UnlockGroup(g *Group) {
	s.UnlockEntries(g.Entries)
	s.UnlockGroups(g.Groups)
}
func (s *SalsaManager) UnlockEntries(e []Entry) {
	for i, _ := range e {
		s.UnlockEntry(&e[i])
	}
}
func (s *SalsaManager) UnlockEntry(e *Entry) {
	for i, _ := range e.Values {
		if bool(e.Values[i].Value.Protected) {
			e.Values[i].Value.Content = string(s.Unpack(e.Values[i].Value.Content))
		}
	}
	for i, _ := range e.Histories {
		s.UnlockEntries(e.Histories[i].Entries)
	}
}

func (s *SalsaManager) LockGroups(gs []Group) {
	for i, _ := range gs {
		s.LockGroup(&gs[i])
	}
}
func (s *SalsaManager) LockGroup(g *Group) {
	s.LockEntries(g.Entries)
	s.LockGroups(g.Groups)
}
func (s *SalsaManager) LockEntries(es []Entry) {
	for i, _ := range es {
		s.LockEntry(&es[i])
	}
}
func (s *SalsaManager) LockEntry(e *Entry) {
	for i, _ := range e.Values {
		if bool(e.Values[i].Value.Protected) {
			e.Values[i].Value.Content = s.Pack([]byte(e.Values[i].Value.Content))
		}
	}
	for i, _ := range e.Histories {
		s.UnlockEntries(e.Histories[i].Entries)
	}
}

func u8to32little(k []byte, i int) uint32 {
	return uint32(k[i]) |
		(uint32(k[i+1]) << 8) |
		(uint32(k[i+2]) << 16) |
		(uint32(k[i+3]) << 24)
}

func rotl32(x uint32, b uint) uint32 {
	return ((x << b) | (x >> (32 - b)))
}

// NewSalsaManager initializes a new Password
func NewSalsaManager(key [32]byte) *SalsaManager {
	state := make([]uint32, 16)

	state[1] = u8to32little(key[:], 0)
	state[2] = u8to32little(key[:], 4)
	state[3] = u8to32little(key[:], 8)
	state[4] = u8to32little(key[:], 12)
	state[11] = u8to32little(key[:], 16)
	state[12] = u8to32little(key[:], 20)
	state[13] = u8to32little(key[:], 24)
	state[14] = u8to32little(key[:], 28)
	state[0] = sigmaWords[0]
	state[5] = sigmaWords[1]
	state[10] = sigmaWords[2]
	state[15] = sigmaWords[3]

	state[6] = u8to32little(iv, 0)
	state[7] = u8to32little(iv, 4)
	state[8] = uint32(0)
	state[9] = uint32(0)

	s := SalsaManager{
		State:        state,
		currentBlock: make([]byte, 0),
	}
	s.reset()
	return &s
}

func (s *SalsaManager) Unpack(payload string) []byte {
	var result []byte

	data, _ := base64.StdEncoding.DecodeString(payload)

	salsaBytes := s.fetchBytes(len(data))

	for i := 0; i < len(data); i++ {
		result = append(result, salsaBytes[i]^data[i])
	}

	return result
}

func (s *SalsaManager) Pack(payload []byte) string {
	var data []byte

	salsaBytes := s.fetchBytes(len(payload))

	for i := 0; i < len(payload); i++ {
		data = append(data, salsaBytes[i]^payload[i])
	}

	lockedPassword := base64.StdEncoding.EncodeToString(data)
	return lockedPassword
}

func (s *SalsaManager) reset() {
	s.blockUsed = 64
	s.counterWords = [2]int{0, 0}
}

func (s *SalsaManager) incrementCounter() {
	s.counterWords[0] = (s.counterWords[0] + 1) & 0xffffffff
	if s.counterWords[0] == 0 {
		s.counterWords[1] = (s.counterWords[1] + 1) & 0xffffffff
	}
}

func (s *SalsaManager) fetchBytes(length int) []byte {
	for length > len(s.currentBlock) {
		s.currentBlock = append(s.currentBlock, s.getBytes(64)...)
	}

	data := s.currentBlock[0:length]
	s.currentBlock = s.currentBlock[length:]

	return data
}

func (s *SalsaManager) getBytes(length int) []byte {
	b := make([]byte, length)

	for i := 0; i < length; i++ {
		if s.blockUsed == 64 {
			s.generateBlock()
			s.incrementCounter()
			s.blockUsed = 0
		}
		b[i] = s.block[s.blockUsed]
		s.blockUsed++
	}

	return b
}

func (s *SalsaManager) generateBlock() {
	s.block = make([]byte, 64)

	x := make([]uint32, 16)
	copy(x, s.State)

	for i := 0; i < 10; i++ {
		x[4] = x[4] ^ rotl32(x[0]+x[12], 7)
		x[8] = x[8] ^ rotl32(x[4]+x[0], 9)
		x[12] = x[12] ^ rotl32(x[8]+x[4], 13)
		x[0] = x[0] ^ rotl32(x[12]+x[8], 18)

		x[9] = x[9] ^ rotl32(x[5]+x[1], 7)
		x[13] = x[13] ^ rotl32(x[9]+x[5], 9)
		x[1] = x[1] ^ rotl32(x[13]+x[9], 13)
		x[5] = x[5] ^ rotl32(x[1]+x[13], 18)

		x[14] = x[14] ^ rotl32(x[10]+x[6], 7)
		x[2] = x[2] ^ rotl32(x[14]+x[10], 9)
		x[6] = x[6] ^ rotl32(x[2]+x[14], 13)
		x[10] = x[10] ^ rotl32(x[6]+x[2], 18)

		x[3] = x[3] ^ rotl32(x[15]+x[11], 7)
		x[7] = x[7] ^ rotl32(x[3]+x[15], 9)
		x[11] = x[11] ^ rotl32(x[7]+x[3], 13)
		x[15] = x[15] ^ rotl32(x[11]+x[7], 18)

		x[1] = x[1] ^ rotl32(x[0]+x[3], 7)
		x[2] = x[2] ^ rotl32(x[1]+x[0], 9)
		x[3] = x[3] ^ rotl32(x[2]+x[1], 13)
		x[0] = x[0] ^ rotl32(x[3]+x[2], 18)

		x[6] = x[6] ^ rotl32(x[5]+x[4], 7)
		x[7] = x[7] ^ rotl32(x[6]+x[5], 9)
		x[4] = x[4] ^ rotl32(x[7]+x[6], 13)
		x[5] = x[5] ^ rotl32(x[4]+x[7], 18)

		x[11] = x[11] ^ rotl32(x[10]+x[9], 7)
		x[8] = x[8] ^ rotl32(x[11]+x[10], 9)
		x[9] = x[9] ^ rotl32(x[8]+x[11], 13)
		x[10] = x[10] ^ rotl32(x[9]+x[8], 18)

		x[12] = x[12] ^ rotl32(x[15]+x[14], 7)
		x[13] = x[13] ^ rotl32(x[12]+x[15], 9)
		x[14] = x[14] ^ rotl32(x[13]+x[12], 13)
		x[15] = x[15] ^ rotl32(x[14]+x[13], 18)
	}

	for i := 0; i < 16; i++ {
		x[i] += s.State[i]
	}

	for i := 0; i < 16; i++ {
		s.block[i<<2] = byte(x[i])
		s.block[(i<<2)+1] = byte(x[i] >> 8)
		s.block[(i<<2)+2] = byte(x[i] >> 16)
		s.block[(i<<2)+3] = byte(x[i] >> 24)
	}
	s.blockUsed = 0
	s.State[8]++
	if s.State[8] == 0 {
		s.State[9]++
	}
}
