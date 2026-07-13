package events

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/TerminalAddict/golive-nms/internal/store"
	"github.com/gosnmp/gosnmp"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Receiver struct {
	store                         *store.Store
	logger                        *slog.Logger
	syslogUDP, syslogTCP, trapUDP string
}

func New(s *store.Store, l *slog.Logger, sysUDP, sysTCP, trap string) *Receiver {
	return &Receiver{store: s, logger: l, syslogUDP: sysUDP, syslogTCP: sysTCP, trapUDP: trap}
}
func (r *Receiver) Run(ctx context.Context) {
	var wg sync.WaitGroup
	wg.Add(3)
	go func() { defer wg.Done(); r.runSyslogUDP(ctx) }()
	go func() { defer wg.Done(); r.runSyslogTCP(ctx) }()
	go func() { defer wg.Done(); r.runTraps(ctx) }()
	<-ctx.Done()
	wg.Wait()
}
func (r *Receiver) runSyslogUDP(ctx context.Context) {
	conn, err := net.ListenPacket("udp", r.syslogUDP)
	if err != nil {
		r.logger.Error("syslog UDP", "error", err)
		return
	}
	defer conn.Close()
	go closeOnDone(ctx, conn)
	buf := make([]byte, 65535)
	for {
		n, addr, e := conn.ReadFrom(buf)
		if e != nil {
			return
		}
		r.recordSyslog(ctx, host(addr), string(buf[:n]))
	}
}
func (r *Receiver) runSyslogTCP(ctx context.Context) {
	listener, err := net.Listen("tcp", r.syslogTCP)
	if err != nil {
		r.logger.Error("syslog TCP", "error", err)
		return
	}
	defer listener.Close()
	go func() { <-ctx.Done(); listener.Close() }()
	for {
		conn, e := listener.Accept()
		if e != nil {
			return
		}
		go func(c net.Conn) {
			defer c.Close()
			scanner := bufio.NewScanner(c)
			scanner.Buffer(make([]byte, 4096), 1<<20)
			for scanner.Scan() {
				r.recordSyslog(ctx, host(c.RemoteAddr()), scanner.Text())
			}
		}(conn)
	}
}
func (r *Receiver) recordSyslog(ctx context.Context, source, message string) {
	facility, severity, body := parsePRI(message)
	e := store.DeviceEvent{Protocol: "syslog", Source: source, Facility: facility, Severity: severity, Message: body, Fields: map[string]any{"raw": message}}
	if err := r.store.RecordDeviceEvent(ctx, e); err != nil {
		r.logger.Warn("store syslog", "error", err)
	}
}
func (r *Receiver) runTraps(ctx context.Context) {
	conn, err := net.ListenPacket("udp", r.trapUDP)
	if err != nil {
		r.logger.Error("SNMP traps", "error", err)
		return
	}
	defer conn.Close()
	go closeOnDone(ctx, conn)
	buf := make([]byte, 65535)
	decoder := &gosnmp.GoSNMP{}
	for {
		n, addr, e := conn.ReadFrom(buf)
		if e != nil {
			return
		}
		packet, e := decoder.UnmarshalTrap(buf[:n], true)
		if e != nil {
			r.logger.Warn("decode SNMP trap", "source", addr, "error", e)
			continue
		}
		fields := map[string]any{"version": fmt.Sprint(packet.Version), "community": packet.Community}
		vars := map[string]string{}
		for _, v := range packet.Variables {
			vars[v.Name] = fmt.Sprint(gosnmp.ToBigInt(v.Value))
		}
		fields["variables"] = vars
		raw, _ := json.Marshal(vars)
		event := store.DeviceEvent{Protocol: "snmp_trap", Source: host(addr), Message: string(raw), Fields: fields}
		if e = r.store.RecordDeviceEvent(ctx, event); e != nil {
			r.logger.Warn("store SNMP trap", "error", e)
		}
	}
}
func parsePRI(message string) (facility, severity *int, body string) {
	body = message
	if !strings.HasPrefix(message, "<") {
		return
	}
	end := strings.IndexByte(message, '>')
	if end < 2 {
		return
	}
	v, err := strconv.Atoi(message[1:end])
	if err != nil || v < 0 || v > 191 {
		return
	}
	f, s := v/8, v%8
	facility, severity = &f, &s
	body = strings.TrimSpace(message[end+1:])
	return
}
func host(a net.Addr) string {
	h, _, e := net.SplitHostPort(a.String())
	if e == nil {
		return h
	}
	return a.String()
}
func closeOnDone(ctx context.Context, c net.PacketConn) {
	<-ctx.Done()
	_ = c.SetDeadline(time.Now())
	_ = c.Close()
}
