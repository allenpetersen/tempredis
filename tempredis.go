package main

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"syscall"
	"time"
)


var (
	RedisStartupSuccess = "The server is now ready to accept connections"
	RedisStartupTimeout = time.Second
)

type Server struct {
	Config Config
	cmd    *exec.Cmd
}

type Config map[string]string

func NewServer(config Config) (server *Server) {
	return &Server{
		Config: config,
	}
}

func (s *Server) Start() (err error) {
	if s.cmd != nil {
		return fmt.Errorf("Already started")
	}
	s.cmd = exec.Command("redis-server", "-")
	stdin, err := s.cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := s.cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err = s.cmd.Start(); err != nil {
		return err
	}
	if err = s.writeConfig(stdin); err != nil {
		return err
	}
	if err = s.waitForSuccessfulStartup(stdout); err != nil {
		s.Stop()
		return err
	}
	return nil
}

func (s *Server) Stop() (err error) {
	s.cmd.Process.Signal(syscall.SIGTERM)
	_, err = s.cmd.Process.Wait()
	if err != nil {
		return err
	}
	return nil
}

func (s *Server) writeConfig(w io.WriteCloser) (err error) {
	for key, value := range s.Config {
		_, err = fmt.Fprintf(w, "%s %s\n", key, value)
		if err != nil {
			return err
		}
	}
	return w.Close()
}

func (s *Server) waitForSuccessfulStartup(r io.ReadCloser) (err error) {
	scanner := bufio.NewScanner(r)
	line := ""

	success := make(chan bool, 1)
	failure := make(chan bool, 1)
	stopWaiting := make(chan bool, 1)

	go func() {
		for {
			select {
			case <-stopWaiting:
				return
			default:
				if scanner.Scan() {
					line = scanner.Text()
					if strings.Contains(line, RedisStartupSuccess) {
						success <- true
						return
					}
				} else {
					failure <- true
					return
				}
			}
		}
	}()

	select {
	case <-success:
		return nil
	case <-failure:
		if err = scanner.Err(); err != nil {
			return fmt.Errorf("Couldn't read redis-server's stdout: %s", err.Error())
		} else {
			return fmt.Errorf("redis-server failed to start up: %s", line)
		}
	case <-time.After(RedisStartupTimeout):
		stopWaiting <- true
		return fmt.Errorf("Timed-out waiting for redis-server to start up successfully. Last line received was: %s", line)
	}
}
