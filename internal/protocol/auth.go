package protocol

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
)

type AuthType int32

const (
	AuthOK                AuthType = 0  // Authentication successful
	AuthKerberosV5        AuthType = 2  // Kerberos V5
	AuthCleartextPassword AuthType = 3  // Cleartext password
	AuthMD5Password       AuthType = 5  // MD5 password
	AuthSCMCredential     AuthType = 6  // SCM credential
	AuthGSS               AuthType = 7  // GSS
	AuthGSSContinue       AuthType = 8  // GSS continue
	AuthSSPI              AuthType = 9  // SSPI
	AuthSASL              AuthType = 10 // SASL
	AuthSASLContinue      AuthType = 11 // SASL continue
	AuthSASLFinal         AuthType = 12 // SASL final
)

type AuthenticationMessage struct {
	Type AuthType
	Data []byte
}

func ParseAuthenticationMessage(data []byte) (*AuthenticationMessage, error) {
	if len(data) < 4 {
		return nil, nil
	}

	authType := AuthType(binary.BigEndian.Uint32(data[:4]))

	msg := &AuthenticationMessage{
		Type: authType,
	}

	if len(data) > 4 {
		msg.Data = data[4:]
	}

	return msg, nil
}

func EncodeAuthenticationOK() []byte {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, uint32(AuthOK))
	return buf
}

func EncodeAuthenticationCleartext() []byte {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, uint32(AuthCleartextPassword))
	return buf
}

func EncodeAuthenticationMD5(salt [4]byte) []byte {
	var buf bytes.Buffer
	typeBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(typeBuf, uint32(AuthMD5Password))
	buf.Write(typeBuf)
	buf.Write(salt[:])
	return buf.Bytes()
}

func EncodeMD5Password(username, password string, salt [4]byte) []byte {
	hash1 := md5.Sum([]byte(password + username))
	hex1 := hex.EncodeToString(hash1[:])

	hash2 := md5.Sum(append([]byte(hex1), salt[:]...))
	hex2 := hex.EncodeToString(hash2[:])

	return append([]byte("md5"+hex2), 0)
}
