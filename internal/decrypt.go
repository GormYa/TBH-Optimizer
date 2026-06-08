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

func LoadSave() (*InnerSaveData, error) {
	caminhoSave := os.Getenv("USERPROFILE") + `\AppData\LocalLow\TesseractStudio\TaskbarHero\SaveFile_Live.es3`
	dados, err := os.ReadFile(caminhoSave)
	if err != nil {
		return nil, err
	}

	if len(dados) <= 16 {
		fmt.Println("Erro: Arquivo pequeno demais para ser um save válido.")
		return nil, fmt.Errorf("arquivo curto demais")
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
		fmt.Println("Erro ao criar a cifra AES:", err)
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

	var inner InnerSaveData
	if err := json.Unmarshal([]byte(outer.PlayerSaveData.Value), &inner); err != nil {
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
