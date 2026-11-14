package runtime_test

import (
	"runtime"
	"testing"

	net "github.com/cloudruntime/mini/src/network"
)

func TestIPAM_Allocate(t *testing.T) {
	ipam, err := net.NewIPAM("10.0.0.0/24", "10.0.0.1")
	if err != nil {
		t.Fatal(err)
	}

	// 第一个分配的 IP 应该是 10.0.0.2（.1 是 gateway）
	ip1, err := ipam.Allocate()
	if err != nil {
		t.Fatal(err)
	}
	if ip1 != "10.0.0.2" {
		t.Fatalf("expected 10.0.0.2, got %s", ip1)
	}

	// 第二个应该是 10.0.0.3
	ip2, err := ipam.Allocate()
	if err != nil {
		t.Fatal(err)
	}
	if ip2 != "10.0.0.3" {
		t.Fatalf("expected 10.0.0.3, got %s", ip2)
	}
}

func TestIPAM_Release(t *testing.T) {
	ipam, _ := net.NewIPAM("10.0.0.0/24", "10.0.0.1")

	ip1, _ := ipam.Allocate()
	ip2, _ := ipam.Allocate()

	if ip1 != "10.0.0.2" || ip2 != "10.0.0.3" {
		t.Fatalf("unexpected IPs: %s, %s", ip1, ip2)
	}

	// 释放 ip1
	ipam.Release(ip1)

	// 下一次分配应该复用释放的 IP
	ip3, _ := ipam.Allocate()
	if ip3 != "10.0.0.2" {
		t.Fatalf("expected reuse of 10.0.0.2, got %s", ip3)
	}
}

func TestIPAM_GatewayNotAllocated(t *testing.T) {
	ipam, _ := net.NewIPAM("10.0.0.0/24", "10.0.0.1")

	if !ipam.IsAllocated("10.0.0.1") {
		t.Fatal("gateway should be marked as allocated")
	}

	ip, _ := ipam.Allocate()
	if ip == "10.0.0.1" {
		t.Fatal("gateway should never be allocated to a sandbox")
	}
}

func TestIPAM_InvalidSubnet(t *testing.T) {
	_, err := net.NewIPAM("invalid", "10.0.0.1")
	if err == nil {
		t.Fatal("expected error for invalid subnet")
	}
}

func TestIPAM_InvalidGateway(t *testing.T) {
	_, err := net.NewIPAM("10.0.0.0/24", "invalid")
	if err == nil {
		t.Fatal("expected error for invalid gateway")
	}
}

func TestNetworkManager_Connect_NonLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("this test only runs on non-Linux")
	}

	mgr, err := net.NewNetworkManager(net.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}

	_, err = mgr.Connect("test-sandbox", nil)
	if err == nil {
		t.Fatal("expected error on non-Linux")
	}
}

func TestNetworkManager_Disconnect_NotFound(t *testing.T) {
	mgr, _ := net.NewNetworkManager(net.DefaultConfig())

	err := mgr.Disconnect("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent sandbox")
	}
}

func TestNetworkManager_GetSandboxNetwork_NotFound(t *testing.T) {
	mgr, _ := net.NewNetworkManager(net.DefaultConfig())

	_, err := mgr.GetSandboxNetwork("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent sandbox")
	}
}

func TestNetworkManager_ListConnected_Empty(t *testing.T) {
	mgr, _ := net.NewNetworkManager(net.DefaultConfig())

	list := mgr.ListConnected()
	if len(list) != 0 {
		t.Fatalf("expected empty list, got %d", len(list))
	}
}

func TestNetworkManager_IsAllowed_NoPolicy(t *testing.T) {
	mgr, _ := net.NewNetworkManager(net.DefaultConfig())

	// 没有策略时，默认允许所有
	if !mgr.IsAllowed("sandbox-a", "sandbox-b") {
		t.Fatal("expected allowed when no policy exists")
	}
}

func TestNetworkManager_IsAllowed_WithPolicy(t *testing.T) {
	mgr, _ := net.NewNetworkManager(net.DefaultConfig())

	mgr.AddPolicy(&net.NetworkPolicy{
		Name:      "policy-1",
		Sandbox:   "sandbox-b",
		AllowFrom: []string{"sandbox-a"},
	})

	// sandbox-a 被允许访问 sandbox-b
	if !mgr.IsAllowed("sandbox-a", "sandbox-b") {
		t.Fatal("expected sandbox-a allowed to access sandbox-b")
	}

	// sandbox-c 不被允许访问 sandbox-b
	if mgr.IsAllowed("sandbox-c", "sandbox-b") {
		t.Fatal("expected sandbox-c NOT allowed to access sandbox-b")
	}
}

func TestNetworkManager_IsAllowed_Wildcard(t *testing.T) {
	mgr, _ := net.NewNetworkManager(net.DefaultConfig())

	mgr.AddPolicy(&net.NetworkPolicy{
		Name:      "policy-1",
		Sandbox:   "sandbox-b",
		AllowFrom: []string{"*"},
	})

	if !mgr.IsAllowed("anyone", "sandbox-b") {
		t.Fatal("expected wildcard to allow all")
	}
}

func TestNetworkManager_RemovePolicy(t *testing.T) {
	mgr, _ := net.NewNetworkManager(net.DefaultConfig())

	mgr.AddPolicy(&net.NetworkPolicy{
		Name:      "policy-1",
		Sandbox:   "sandbox-b",
		AllowFrom: []string{"sandbox-a"},
	})

	if !mgr.IsAllowed("sandbox-a", "sandbox-b") {
		t.Fatal("expected allowed before policy removal")
	}

	mgr.RemovePolicy("policy-1")

	// 移除策略后，默认允许
	if !mgr.IsAllowed("sandbox-c", "sandbox-b") {
		t.Fatal("expected allowed after policy removal")
	}
}

func TestNetworkManager_ListPolicies(t *testing.T) {
	mgr, _ := net.NewNetworkManager(net.DefaultConfig())

	mgr.AddPolicy(&net.NetworkPolicy{
		Name:    "policy-1",
		Sandbox: "sandbox-b",
	})

	mgr.AddPolicy(&net.NetworkPolicy{
		Name:    "policy-2",
		Sandbox: "sandbox-c",
	})

	policies := mgr.ListPolicies()
	if len(policies) != 2 {
		t.Fatalf("expected 2 policies, got %d", len(policies))
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := net.DefaultConfig()
	if cfg.Subnet != "10.0.0.0/24" {
		t.Fatalf("expected subnet 10.0.0.0/24, got %s", cfg.Subnet)
	}
	if cfg.Gateway != "10.0.0.1" {
		t.Fatalf("expected gateway 10.0.0.1, got %s", cfg.Gateway)
	}
	if cfg.Bridge != "cbr0" {
		t.Fatalf("expected bridge cbr0, got %s", cfg.Bridge)
	}
}
