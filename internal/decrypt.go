package internal

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"os"

	"golang.org/x/crypto/pbkdf2"
)

// es3Password é a senha estática do ES3 (Easy Save 3) usada pelo jogo. NÃO fica no
// código-fonte (repo público): é injetada no build via
//   go build -ldflags "-X optimizer/internal.es3Password=<senha>"
// No CI ela vem de um GitHub Secret; pra rodar local, defina a env TBH_ES3_KEY.
var es3Password string

func savePassword() string {
	if es3Password != "" {
		return es3Password
	}
	return os.Getenv("TBH_ES3_KEY")
}

// SavePath devolve o caminho do save ao vivo do jogo.
func SavePath() string {
	return os.Getenv("USERPROFILE") + `\AppData\LocalLow\TesseractStudio\TaskbarHero\SaveFile_Live.es3`
}

// DecryptSaveInner decripta o .es3 e devolve o JSON INTERNO cru (o conteudo de
// PlayerSaveData.value, ja desserializado do envelope ES3). E a fonte unica de verdade
// para LoadSave e para ferramentas que precisam inspecionar campos ainda nao mapeados
// (ex.: cronometro de estagio, pets, runas).
func DecryptSaveInner() ([]byte, error) {
	dados, err := os.ReadFile(SavePath())
	if err != nil {
		return nil, err
	}
	if len(dados) <= 16 {
		return nil, fmt.Errorf("arquivo curto demais para ser um save valido")
	}

	salt := dados[:16]
	iv := salt
	textoCriptografado := dados[16:]

	senhaRaiz := savePassword()
	if senhaRaiz == "" {
		return nil, fmt.Errorf("chave de descriptografia ausente: defina a env TBH_ES3_KEY ou compile com -ldflags \"-X optimizer/internal.es3Password=...\"")
	}

	chaveDerivada := pbkdf2.Key([]byte(senhaRaiz), salt, 100, 16, sha1.New)
	block, err := aes.NewCipher(chaveDerivada)
	if err != nil {
		return nil, err
	}
	mode := cipher.NewCBCDecrypter(block, iv)
	textoDescriptografado := make([]byte, len(textoCriptografado))
	mode.CryptBlocks(textoDescriptografado, textoCriptografado)
	textoLimpo := removerPadding(textoDescriptografado)

	var outer OuterSave
	if err := json.Unmarshal(textoLimpo, &outer); err != nil {
		return nil, err
	}
	return []byte(outer.PlayerSaveData.Value), nil
}

func LoadSave() (*InnerSaveData, error) {
	innerJSON, err := DecryptSaveInner()
	if err != nil {
		return nil, err
	}
	var inner InnerSaveData
	if err := json.Unmarshal(innerJSON, &inner); err != nil {
		return nil, err
	}
	return &inner, nil
}

func removerPadding(src []byte) []byte {
	length := len(src)
	if length == 0 {
		return src
	}
	unpadding := int(src[length-1])
	if unpadding > length || unpadding > aes.BlockSize {
		return src
	}
	return src[:(length - unpadding)]
}
