package game

import (
	"encoding/binary"
	"fmt"

	"golang-refine/internal/config"
)

type API struct {
	cfg    *config.Config
	gamed  *Gamed
	online bool
}

func NewAPI(cfg *config.Config) *API {
	a := &API{
		cfg:   cfg,
		gamed: NewGamed(cfg),
	}
	a.online = a.gamed.ServerOnline()
	return a
}

func (a *API) ServerOnline() bool {
	return a.gamed.ServerOnline()
}

type RoleBase struct {
	Version      byte
	ID           int32
	Name         string
	Race         int32
	Cls          int32
	Gender       byte
	CustomData   string
	ConfigData   string
	CustomStamp  int32
	Status       byte
	DeleteTime   int32
	CreateTime   int32
	LastLogin    int32
	ForbidCount  uint32
	ForbidType   byte
	ForbidTime   int32
	ForbidCT     int32
	ForbidReason string
	HelpStates   string
	Spouse       int32
	UserID       int32
	CrossData    string
}

func (a *API) GetRoleBase(roleID int) (string, error) {
	pack := make([]byte, 8)
	binary.BigEndian.PutUint32(pack[0:4], uint32(0xFFFFFFFF))
	binary.BigEndian.PutUint32(pack[4:8], uint32(roleID))

	opcode := a.gamed.opcodes["getRoleBase"]
	header := a.gamed.CreateHeader(opcode, pack)

	resp, err := a.gamed.SendToGamedBD(header)
	if err != nil {
		return "", err
	}

	data := a.gamed.DeleteHeader(resp)
	if data == nil || len(data) == 0 {
		return "", fmt.Errorf("empty response from gamedbd")
	}

	role := &RoleBase{}
	p := 0

	role.Version, p = readByte(data, p)
	role.ID, p = readInt32BE(data, p)
	name := unpackString(data, &p)
	role.Race, p = readInt32BE(data, p)
	role.Cls, p = readInt32BE(data, p)
	role.Gender, p = readByte(data, p)
	role.CustomData = unpackOctet(data, &p)
	role.ConfigData = unpackOctet(data, &p)
	role.CustomStamp, p = readInt32BE(data, p)
	role.Status, p = readByte(data, p)
	role.DeleteTime, p = readInt32BE(data, p)
	role.CreateTime, p = readInt32BE(data, p)
	role.LastLogin, p = readInt32BE(data, p)
	role.ForbidCount, p = readCuint(data, p)
	if role.ForbidCount > 0 {
		role.ForbidType, p = readByte(data, p)
		role.ForbidTime, p = readInt32BE(data, p)
		role.ForbidCT, p = readInt32BE(data, p)
		role.ForbidReason = unpackString(data, &p)
	}
	role.HelpStates = unpackOctet(data, &p)
	role.Spouse, p = readInt32BE(data, p)
	role.UserID, p = readInt32BE(data, p)
	role.CrossData = unpackOctet(data, &p)

	return name, nil
}

func readByte(data []byte, p int) (byte, int) {
	if p >= len(data) {
		return 0, p
	}
	return data[p], p + 1
}

func readInt32BE(data []byte, p int) (int32, int) {
	if p+4 > len(data) {
		return 0, p
	}
	v := int32(binary.BigEndian.Uint32(data[p : p+4]))
	return v, p + 4
}

func readCuint(data []byte, p int) (uint32, int) {
	cp := p
	v := unpackCuint(data, &cp)
	return uint32(v), cp
}
