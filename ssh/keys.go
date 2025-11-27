package ssh

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"time"

	"golang.org/x/crypto/ssh"
)

func readFileHelper(path string) (data string, err error) {
	var fileData []byte
	if fileData, err = os.ReadFile(path); err != nil {
		return "", err
	}

	return string(fileData), nil
}

func CreateSSHKeyPair() (pub, priv string, err error) {
	var (
		privateKey                *rsa.PrivateKey
		publicKey                 ssh.PublicKey
		pemBlock                  *pem.Block
		privateKeyFile            *os.File
		pubFileName, privFileName string = fmt.Sprintf("%s/opnlaas-ssh-keygen-%d.pub", os.TempDir(), time.Now().UnixNano()), fmt.Sprintf("%s/opnlaas-ssh-keygen-%d", os.TempDir(), time.Now().UnixNano())
	)

	defer func() {
		os.Remove(pubFileName)
		os.Remove(privFileName)
	}()

	if privateKey, err = rsa.GenerateKey(rand.Reader, 4096); err != nil {
		return
	}

	if privateKeyFile, err = os.Create(privFileName); err != nil {
		return
	}

	defer privateKeyFile.Close()

	if err = privateKeyFile.Chmod(0600); err != nil {
		return
	}

	pemBlock = &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}

	if err = pem.Encode(privateKeyFile, pemBlock); err != nil {
		return
	}

	if publicKey, err = ssh.NewPublicKey(&privateKey.PublicKey); err != nil {
		return
	}

	if err = os.WriteFile(pubFileName, ssh.MarshalAuthorizedKey(publicKey), 0644); err != nil {
		return
	}

	if pub, err = readFileHelper(pubFileName); err != nil {
		return
	}

	if priv, err = readFileHelper(privFileName); err != nil {
		return
	}

	return
}
