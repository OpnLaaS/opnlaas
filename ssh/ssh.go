package ssh

import (
	"fmt"
	"time"

	"golang.org/x/crypto/ssh"
)

type SSHConnection struct {
	client   *ssh.Client
	session  *ssh.Session
	username string
	host     string
	port     int
	auth     ssh.AuthMethod
}

// If you've run a command and want to run another, you need to reset the session
func (conn *SSHConnection) Reset() (err error) {
	if err = conn.session.Close(); err != nil {
		return
	}

	conn.session, err = conn.client.NewSession()
	return
}

func (conn *SSHConnection) Close() (err error) {
	if err = conn.session.Close(); err != nil {
		return
	}

	err = conn.client.Close()
	return
}

func (conn *SSHConnection) Send(command string) (err error) {
	err = conn.session.Run(command)
	return
}

func (conn *SSHConnection) SendWithOutput(command string) (status int, output []byte, err error) {
	if output, err = conn.session.CombinedOutput(command); err != nil {
		var (
			exitErr *ssh.ExitError
			ok      bool
		)

		if exitErr, ok = err.(*ssh.ExitError); ok {
			status = exitErr.ExitStatus()
			err = nil
			return
		}

		status = -1
		return
	}

	status = 0
	return
}

func WithPrivateKey(key []byte) ssh.AuthMethod {
	var (
		signer ssh.Signer
		err    error
	)

	if signer, err = ssh.ParsePrivateKey(key); err != nil {
		panic(err)
	}

	return ssh.PublicKeys(signer)
}

func WithPassword(password string) ssh.AuthMethod {
	return ssh.Password(password)
}

func Connect(username, host string, port int, auth ssh.AuthMethod) (conn *SSHConnection, err error) {
	conn = &SSHConnection{
		username: username,
		host:     host,
		port:     port,
		auth:     auth,
	}

	if conn.client, err = ssh.Dial("tcp", fmt.Sprintf("%s:%d", host, port), &ssh.ClientConfig{
		User:            username,
		Auth:            []ssh.AuthMethod{auth},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}); err != nil {
		return nil, err
	}

	if conn.session, err = conn.client.NewSession(); err != nil {
		conn.client.Close()
		return nil, err
	}

	return conn, nil
}

func ConnectOnceReadyWithRetry(username, host string, port int, auth ssh.AuthMethod, retries int) (conn *SSHConnection, err error) {
	if err = WaitOnline(host); err != nil {
		return
	}

	for i := range retries {
		err = nil
		if conn, err = Connect(username, host, port, auth); err == nil {
			return
		}

		time.Sleep((time.Duration(i) + 1) * time.Second)
	}

	return
}
