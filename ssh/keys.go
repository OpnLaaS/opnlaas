package ssh

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"

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
		privateKey     *rsa.PrivateKey
		publicKey      ssh.PublicKey
		pemBlock       *pem.Block
		privateKeyFile *os.File
	)

	if privateKey, err = rsa.GenerateKey(rand.Reader, 4096); err != nil {
		return
	}

	if privateKeyFile, err = os.Create(os.TempDir() + "/id_rsa"); err != nil {
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

	if err = os.WriteFile(os.TempDir()+"/id_rsa.pub", ssh.MarshalAuthorizedKey(publicKey), 0644); err != nil {
		return
	}

	if pub, err = readFileHelper(os.TempDir() + "/id_rsa.pub"); err != nil {
		return
	}

	if priv, err = readFileHelper(os.TempDir() + "/id_rsa"); err != nil {
		return
	}

	return
}
