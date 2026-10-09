//go:build m5stack

package main

import (
	"errors"
	"net/netip"
	"runtime"
	"time"

	"github.com/soypat/lneto"
	"github.com/soypat/lneto/dns"
	"tinygo.org/x/espradio"
)

var (
	// ssid and password intentionally remain empty so credentials can be
	// supplied at build time without being committed to the repository.
	ssid     string
	password string
)

const (
	ntpHost              = "pool.ntp.org"
	initialRetryDelay    = 30 * time.Second
	maximumRetryDelay    = 5 * time.Minute
	apNotFoundRetryDelay = 5 * time.Second
	resynchronizationIn  = 6 * time.Hour
	wifiReasonNoAPFound  = espradio.Error(201)
)

var pollBackoff = lneto.BackoffStrategy(func(_ uint) time.Duration {
	return 5 * time.Millisecond
})

func runTimeSync(inbox chan<- UIMessage) {
	// Let the UI paint its first frame before radio initialization begins.
	time.Sleep(time.Second)

	ensureRadioStarted(inbox)

	retryDelay := initialRetryDelay
	for {
		inbox <- UIMessage{Kind: UIMessageSyncChanged, Sync: Syncing}
		session := connectNetworkWithRetry(inbox)

		offset, err := queryNTP(session.stack)
		session.close()

		synced := err == nil
		if synced {
			runtime.AdjustTimeOffset(int64(offset))
			println("time synchronized:", time.Now().String())
			inbox <- UIMessage{Kind: UIMessageSyncChanged, Sync: Synced}
			retryDelay = initialRetryDelay
		} else {
			println("NTP synchronization failed:", err.Error())
			inbox <- UIMessage{Kind: UIMessageSyncChanged, Sync: SyncFailed}
		}

		// Nothing touches the radio until the next attempt, so power it down
		// for the whole wait instead of keeping the association alive. Every
		// cycle then starts from a fresh association and DHCP lease, which
		// also sidesteps silent drops of long-lived Wi-Fi connections.
		stopRadio()
		if synced {
			time.Sleep(resynchronizationIn)
		} else {
			time.Sleep(retryDelay)
			retryDelay = nextRetryDelay(retryDelay)
		}
		startRadio()
	}
}

// ensureRadioStarted performs the one-time radio bring-up. Enable and Start
// have no teardown path in espradio, so their failures are terminal: report
// the failure at a bounded rate and keep the clock itself alive.
func ensureRadioStarted(inbox chan<- UIMessage) {
	inbox <- UIMessage{Kind: UIMessageSyncChanged, Sync: Syncing}
	println("initializing radio...")
	if err := espradio.Enable(espradio.Config{Logging: espradio.LogLevelError}); err != nil {
		// Enable has no inverse and deliberately rejects a second call. Keep the
		// clock alive and report the permanent radio failure at a bounded rate.
		for {
			println("radio initialization failed:", err.Error())
			inbox <- UIMessage{Kind: UIMessageSyncChanged, Sync: SyncFailed}
			time.Sleep(maximumRetryDelay)
		}
	}

	if err := espradio.Start(); err != nil {
		// Like Enable, Start has no teardown API. Avoid unsafe repeated partial
		// initialization while keeping UI timekeeping responsive.
		for {
			println("radio start failed:", err.Error())
			inbox <- UIMessage{Kind: UIMessageSyncChanged, Sync: SyncFailed}
			time.Sleep(maximumRetryDelay)
		}
	}
}

// stopRadio powers the Wi-Fi driver down until the next startRadio. A failed
// stop is not fatal by itself: startRadio recovers by stopping again.
func stopRadio() {
	println("stopping radio...")
	if err := espradio.Stop(); err != nil {
		println("radio stop failed:", err.Error())
	} else {
		println("radio stopped")
	}
}

// startRadio brings the Wi-Fi driver back up after stopRadio. If the driver
// never came down cleanly, Start reports NOT_STOPPED; stop again and retry
// rather than looping tightly.
func startRadio() {
	println("starting radio...")
	for {
		err := espradio.Start()
		if err == nil {
			println("radio started")
			return
		}
		println("radio start failed:", err.Error())
		stopRadio()
		time.Sleep(apNotFoundRetryDelay)
	}
}

func connectNetworkWithRetry(inbox chan<- UIMessage) *networkSession {
	retryDelay := initialRetryDelay
	for {
		inbox <- UIMessage{Kind: UIMessageSyncChanged, Sync: Syncing}
		session, err := connectNetwork()
		if err == nil {
			return session
		}

		println("network initialization failed:", err.Error())
		inbox <- UIMessage{Kind: UIMessageSyncChanged, Sync: SyncFailed}
		if isAPNotFound(err) {
			println("retrying WiFi connection in", apNotFoundRetryDelay.String())
			time.Sleep(apNotFoundRetryDelay)
			continue
		}
		println("retrying network initialization in", retryDelay.String())
		time.Sleep(retryDelay)
		retryDelay = nextRetryDelay(retryDelay)
	}
}

func isAPNotFound(err error) bool {
	var radioErr espradio.Error
	return errors.As(err, &radioErr) && radioErr == wifiReasonNoAPFound
}

// networkSession owns the per-cycle network resources: the lneto stack and
// the goroutine pumping it. close must be called before stopping the radio so
// no goroutine touches a stopped driver.
type networkSession struct {
	stack *espradio.Stack
	stop  chan struct{}
	done  chan struct{}
}

func (s *networkSession) close() {
	if s.stop == nil {
		return
	}
	close(s.stop)
	<-s.done
	s.stop = nil
}

func connectNetwork() (*networkSession, error) {
	println("connecting WiFi...")
	if err := espradio.Connect(espradio.STAConfig{
		SSID:     ssid,
		Password: password,
	}); err != nil {
		return nil, err
	}

	device, err := espradio.StartNetDev()
	if err != nil {
		return nil, err
	}
	stack, err := espradio.NewStack(device, espradio.StackConfig{
		Hostname:    "m5stack-fire",
		MaxUDPPorts: 2,
		MaxTCPPorts: 1,
	})
	if err != nil {
		return nil, err
	}

	session := &networkSession{
		stack: stack,
		stop:  make(chan struct{}),
		done:  make(chan struct{}),
	}
	go pumpNetwork(stack, session.stop, session.done)

	println("requesting DHCP lease...")
	dhcp, err := stack.SetupWithDHCP(espradio.DHCPConfig{})
	if err != nil {
		session.close()
		return nil, err
	}
	addr, ok := netip.AddrFromSlice(dhcp.AssignedAddr4[:])
	if !ok {
		session.close()
		return nil, errors.New("DHCP returned an invalid IPv4 address")
	}
	println("IP:", addr.String())
	return session, nil
}

func pumpNetwork(stack *espradio.Stack, stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	for {
		select {
		case <-stop:
			return
		default:
		}

		send, receive, err := stack.RecvAndSend()
		if err != nil {
			println("network pump:", err.Error())
		}
		if send == 0 && receive == 0 {
			time.Sleep(5 * time.Millisecond)
		}
	}
}

func queryNTP(stack *espradio.Stack) (time.Duration, error) {
	retryingStack := stack.LnetoStack().StackRetrying(pollBackoff)
	println("resolving", ntpHost)
	addresses, err := retryingStack.DoLookupIP(
		dns.MustNewName(ntpHost), 5*time.Second, 3,
	)
	if err != nil {
		return 0, err
	}

	var lastErr error
	for _, addr := range addresses {
		println("NTP candidate:", addr.String())
		offset, err := retryingStack.DoNTP(addr, 5*time.Second, 1)
		if err == nil {
			println("NTP success:", addr.String())
			return offset, nil
		}
		lastErr = err
		println("NTP candidate failed:", addr.String(), err.Error())
	}
	if lastErr != nil {
		return 0, lastErr
	}
	return 0, errors.New("DNS returned no NTP candidates")
}

func nextRetryDelay(current time.Duration) time.Duration {
	next := current * 2
	if next > maximumRetryDelay {
		return maximumRetryDelay
	}
	return next
}
