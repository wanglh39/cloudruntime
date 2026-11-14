package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	apipkg "github.com/cloudruntime/mini/src/api"
	faaspkg "github.com/cloudruntime/mini/src/faas"
	netpkg "github.com/cloudruntime/mini/src/network"
	"github.com/cloudruntime/mini/src/runtime"
	"github.com/cloudruntime/mini/src/scheduler"
	stpkg "github.com/cloudruntime/mini/src/storage"
)

const (
	banner = `
  ___ _                 _ _____                    _       
 / __\ | ___  _   _  __| /__   \_   _  __ _ _ __ __| | ___  
/ /  | |/ _ \| | | |/ _` + "`" + `  / /\/ | | |/ _\` + "`" + ` | '__/ _\` + "`" + ` |/ _ \ 
/ /__| | (_) | |_| | (_| |/ /  | |_| | (_| | | | (_| | (_) |
\____/_|\___/ \__,_|\__,_\/   \__, |\__,_|_|  \__,_|\___/ 
                              |___/                        
  Mini Cloud Runtime — 教学项目
  从零理解云运行时的底层原理与设计哲学
`
)

var sandboxMgr = runtime.NewSandboxManager()
var sched = scheduler.NewScheduler()
var netMgr, _ = netpkg.NewNetworkManager(netpkg.DefaultConfig())
var storageMgr = stpkg.NewStorageManager()
var faasEngine = faaspkg.NewFaaSEngine()
var apiStore = apipkg.NewResourceStore()
var apiServer = apipkg.NewAPIServer(apiStore)
var ctrlMgr = apipkg.NewControllerManager(apiStore)

func main() {
	fmt.Print(banner)

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "version":
		fmt.Println("cloudruntime v0.0.1 (教学版)")
	case "sandbox":
		handleSandbox(os.Args[2:])
	case "scheduler":
		handleScheduler(os.Args[2:])
	case "network":
		handleNetwork(os.Args[2:])
	case "storage":
		handleStorage(os.Args[2:])
	case "faas":
		handleFaaS(os.Args[2:])
	case "api":
		handleAPI(os.Args[2:])
	case "help":
		printUsage()
	default:
		fmt.Printf("未知命令: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func handleSandbox(args []string) {
	if len(args) == 0 {
		fmt.Println(`sandbox 子命令:
  sandbox create <name> --cpu <millicores> --mem <bytes> -- <command...>
  sandbox list
  sandbox stop <name>
  sandbox delete <name>
  sandbox inspect <name>`)
		return
	}

	switch args[0] {
	case "create":
		handleSandboxCreate(args[1:])
	case "list":
		handleSandboxList()
	case "stop":
		if len(args) < 2 {
			fmt.Println("用法: sandbox stop <name>")
			return
		}
		if err := sandboxMgr.Stop(args[1]); err != nil {
			fmt.Printf("错误: %v\n", err)
		} else {
			fmt.Printf("沙箱 %s 已停止\n", args[1])
		}
	case "delete":
		if len(args) < 2 {
			fmt.Println("用法: sandbox delete <name>")
			return
		}
		if err := sandboxMgr.Delete(args[1]); err != nil {
			fmt.Printf("错误: %v\n", err)
		} else {
			fmt.Printf("沙箱 %s 已删除\n", args[1])
		}
	case "inspect":
		if len(args) < 2 {
			fmt.Println("用法: sandbox inspect <name>")
			return
		}
		sb, err := sandboxMgr.Get(args[1])
		if err != nil {
			fmt.Printf("错误: %v\n", err)
			return
		}
		data, _ := json.MarshalIndent(sb, "", "  ")
		fmt.Println(string(data))
	default:
		fmt.Printf("未知子命令: %s\n", args[0])
	}
}

func handleSandboxCreate(args []string) {
	var name string
	var cpuLimit int
	var memLimit int64
	var command []string

	i := 0
	for i < len(args) {
		switch args[i] {
		case "--cpu":
			if i+1 < len(args) {
				cpuLimit, _ = strconv.Atoi(args[i+1])
				i += 2
			} else {
				i++
			}
		case "--mem":
			if i+1 < len(args) {
				memLimit, _ = strconv.ParseInt(args[i+1], 10, 64)
				i += 2
			} else {
				i++
			}
		case "--":
			command = args[i+1:]
			i = len(args)
		default:
			if name == "" {
				name = args[i]
			}
			i++
		}
	}

	if name == "" || len(command) == 0 {
		fmt.Println("用法: sandbox create <name> --cpu <millicores> --mem <bytes> -- <command...>")
		return
	}

	spec := runtime.SandboxSpec{
		Name:     name,
		Command:  command,
		CPULimit: cpuLimit,
		MemLimit: memLimit,
	}

	sb, err := sandboxMgr.Create(spec)
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		return
	}

	fmt.Printf("沙箱 %s 已创建 (PID: %d, 状态: %s)\n", sb.Status.Name, sb.Status.PID, sb.Status.State)
}

func handleSandboxList() {
	list := sandboxMgr.List()
	if len(list) == 0 {
		fmt.Println("没有运行中的沙箱")
		return
	}

	fmt.Printf("%-20s %-10s %-8s %-10s\n", "NAME", "STATE", "PID", "CPU(m)")
	for _, sb := range list {
		fmt.Printf("%-20s %-10s %-8d %-10d\n",
			sb.Status.Name, sb.Status.State, sb.Status.PID, sb.Spec.CPULimit)
	}
}

func handleScheduler(args []string) {
	if len(args) == 0 {
		fmt.Println(`scheduler 子命令:
  scheduler node add <name> --cpu <millicores> --mem <bytes> [--label key=value]
  scheduler node list
  scheduler node remove <name>
  scheduler task submit <name> --cpu <millicores> --mem <bytes> [--policy binpack|spread] [--selector key=value]
  scheduler task list
  scheduler task inspect <name>
  scheduler task delete <name>`)
		return
	}

	switch args[0] {
	case "node":
		handleSchedulerNode(args[1:])
	case "task":
		handleSchedulerTask(args[1:])
	default:
		fmt.Printf("未知子命令: %s\n", args[0])
	}
}

func handleSchedulerNode(args []string) {
	if len(args) == 0 {
		fmt.Println("用法: scheduler node <add|list|remove> ...")
		return
	}

	switch args[0] {
	case "add":
		handleNodeAdd(args[1:])
	case "list":
		handleNodeList()
	case "remove":
		if len(args) < 2 {
			fmt.Println("用法: scheduler node remove <name>")
			return
		}
		sched.RemoveNode(args[1])
		fmt.Printf("节点 %s 已移除\n", args[1])
	default:
		fmt.Printf("未知子命令: %s\n", args[0])
	}
}

func handleNodeAdd(args []string) {
	var name string
	var cpu int
	var mem int64
	labels := make(map[string]string)

	i := 0
	for i < len(args) {
		switch args[i] {
		case "--cpu":
			if i+1 < len(args) {
				cpu, _ = strconv.Atoi(args[i+1])
				i += 2
			} else {
				i++
			}
		case "--mem":
			if i+1 < len(args) {
				mem, _ = strconv.ParseInt(args[i+1], 10, 64)
				i += 2
			} else {
				i++
			}
		case "--label":
			if i+1 < len(args) {
				kv := args[i+1]
				for j := 0; j < len(kv); j++ {
					if kv[j] == '=' {
						labels[kv[:j]] = kv[j+1:]
						break
					}
				}
				i += 2
			} else {
				i++
			}
		default:
			if name == "" {
				name = args[i]
			}
			i++
		}
	}

	if name == "" {
		fmt.Println("用法: scheduler node add <name> --cpu <millicores> --mem <bytes> [--label key=value]")
		return
	}

	node := &scheduler.Node{
		Spec: scheduler.NodeSpec{
			Name:   name,
			CPUCap: cpu,
			MemCap: mem,
			Labels: labels,
		},
		Status: scheduler.NodeStatus{Name: name},
	}
	sched.AddNode(node)
	fmt.Printf("节点 %s 已添加 (CPU: %dm, Mem: %d bytes)\n", name, cpu, mem)
}

func handleNodeList() {
	nodes := sched.ListNodes()
	if len(nodes) == 0 {
		fmt.Println("没有节点")
		return
	}

	fmt.Printf("%-15s %-10s %-10s %-10s %-10s\n", "NAME", "CPU_CAP", "CPU_USED", "MEM_CAP", "TASKS")
	for _, n := range nodes {
		fmt.Printf("%-15s %-10d %-10d %-10d %-10d\n",
			n.Spec.Name, n.Spec.CPUCap, n.Status.CPUUsed, n.Spec.MemCap, n.Status.TaskCount)
	}
}

func handleSchedulerTask(args []string) {
	if len(args) == 0 {
		fmt.Println("用法: scheduler task <submit|list|inspect|delete> ...")
		return
	}

	switch args[0] {
	case "submit":
		handleTaskSubmit(args[1:])
	case "list":
		handleTaskList()
	case "inspect":
		if len(args) < 2 {
			fmt.Println("用法: scheduler task inspect <name>")
			return
		}
		task, err := sched.GetTask(args[1])
		if err != nil {
			fmt.Printf("错误: %v\n", err)
			return
		}
		data, _ := json.MarshalIndent(task, "", "  ")
		fmt.Println(string(data))
	case "delete":
		if len(args) < 2 {
			fmt.Println("用法: scheduler task delete <name>")
			return
		}
		sched.DeleteTask(args[1])
		fmt.Printf("任务 %s 已删除\n", args[1])
	default:
		fmt.Printf("未知子命令: %s\n", args[0])
	}
}

func handleTaskSubmit(args []string) {
	var name string
	var cpu int
	var mem int64
	var policy scheduler.SchedulingPolicy
	selector := make(map[string]string)

	i := 0
	for i < len(args) {
		switch args[i] {
		case "--cpu":
			if i+1 < len(args) {
				cpu, _ = strconv.Atoi(args[i+1])
				i += 2
			} else {
				i++
			}
		case "--mem":
			if i+1 < len(args) {
				mem, _ = strconv.ParseInt(args[i+1], 10, 64)
				i += 2
			} else {
				i++
			}
		case "--policy":
			if i+1 < len(args) {
				policy = scheduler.SchedulingPolicy(args[i+1])
				i += 2
			} else {
				i++
			}
		case "--selector":
			if i+1 < len(args) {
				kv := args[i+1]
				for j := 0; j < len(kv); j++ {
					if kv[j] == '=' {
						selector[kv[:j]] = kv[j+1:]
						break
					}
				}
				i += 2
			} else {
				i++
			}
		default:
			if name == "" {
				name = args[i]
			}
			i++
		}
	}

	if name == "" {
		fmt.Println("用法: scheduler task submit <name> --cpu <millicores> --mem <bytes> [--policy binpack|spread] [--selector key=value]")
		return
	}

	task := &scheduler.Task{
		Spec: scheduler.TaskSpec{
			Name:         name,
			CPURequest:   cpu,
			MemRequest:   mem,
			Policy:       policy,
			NodeSelector: selector,
		},
		Status: scheduler.TaskStatus{Name: name},
	}
	sched.SubmitTask(task)

	result, _ := sched.GetTask(name)
	if result.Status.Phase == "scheduled" {
		fmt.Printf("任务 %s 已调度到节点 %s\n", name, result.Status.NodeName)
	} else {
		fmt.Printf("任务 %s 调度失败: %s (phase: %s)\n", name, result.Status.Message, result.Status.Phase)
	}
}

func handleTaskList() {
	tasks := sched.ListTasks()
	if len(tasks) == 0 {
		fmt.Println("没有任务")
		return
	}

	fmt.Printf("%-15s %-10s %-15s %-10s\n", "NAME", "PHASE", "NODE", "POLICY")
	for _, t := range tasks {
		fmt.Printf("%-15s %-10s %-15s %-10s\n",
			t.Spec.Name, t.Status.Phase, t.Status.NodeName, t.Spec.Policy)
	}
}

func handleNetwork(args []string) {
	if len(args) == 0 {
		fmt.Println(`network 子命令:
  network connect <sandbox-name> [--port host:container]
  network disconnect <sandbox-name>
  network list
  network inspect <sandbox-name>
  network policy add <name> --sandbox <name> [--allow-from <name>] [--allow-to <name>]
  network policy list
  network policy remove <name>`)
		return
	}

	switch args[0] {
	case "connect":
		handleNetworkConnect(args[1:])
	case "disconnect":
		if len(args) < 2 {
			fmt.Println("用法: network disconnect <sandbox-name>")
			return
		}
		if err := netMgr.Disconnect(args[1]); err != nil {
			fmt.Printf("错误: %v\n", err)
		} else {
			fmt.Printf("沙箱 %s 网络已断开\n", args[1])
		}
	case "list":
		handleNetworkList()
	case "inspect":
		if len(args) < 2 {
			fmt.Println("用法: network inspect <sandbox-name>")
			return
		}
		sn, err := netMgr.GetSandboxNetwork(args[1])
		if err != nil {
			fmt.Printf("错误: %v\n", err)
			return
		}
		data, _ := json.MarshalIndent(sn, "", "  ")
		fmt.Println(string(data))
	case "policy":
		handleNetworkPolicy(args[1:])
	default:
		fmt.Printf("未知子命令: %s\n", args[0])
	}
}

func handleNetworkConnect(args []string) {
	var sandboxName string
	var portMaps []netpkg.PortMap

	i := 0
	for i < len(args) {
		switch args[i] {
		case "--port":
			if i+1 < len(args) {
				// 格式: hostPort:containerPort
				portStr := args[i+1]
				hostPort, containerPort := 0, 0
				for j := 0; j < len(portStr); j++ {
					if portStr[j] == ':' {
						hostPort, _ = strconv.Atoi(portStr[:j])
						containerPort, _ = strconv.Atoi(portStr[j+1:])
						break
					}
				}
				if hostPort > 0 && containerPort > 0 {
					portMaps = append(portMaps, netpkg.PortMap{
						HostPort:      hostPort,
						ContainerPort: containerPort,
						Protocol:      "tcp",
					})
				}
				i += 2
			} else {
				i++
			}
		default:
			if sandboxName == "" {
				sandboxName = args[i]
			}
			i++
		}
	}

	if sandboxName == "" {
		fmt.Println("用法: network connect <sandbox-name> [--port host:container]")
		return
	}

	sn, err := netMgr.Connect(sandboxName, portMaps)
	if err != nil {
		fmt.Printf("错误: %v\n", err)
		return
	}

	fmt.Printf("沙箱 %s 已连接网络 (IP: %s, veth: %s/%s)\n",
		sn.SandboxName, sn.IP, sn.VethHost, sn.VethSandbox)
	if len(sn.PortMaps) > 0 {
		for _, pm := range sn.PortMaps {
			fmt.Printf("  端口映射: %d → %d (%s)\n", pm.HostPort, pm.ContainerPort, pm.Protocol)
		}
	}
}

func handleNetworkList() {
	list := netMgr.ListConnected()
	if len(list) == 0 {
		fmt.Println("没有已连接网络的沙箱")
		return
	}

	fmt.Printf("%-15s %-15s %-15s %-15s\n", "SANDBOX", "IP", "VETH_HOST", "VETH_SANDBOX")
	for _, sn := range list {
		fmt.Printf("%-15s %-15s %-15s %-15s\n",
			sn.SandboxName, sn.IP, sn.VethHost, sn.VethSandbox)
	}
}

func handleNetworkPolicy(args []string) {
	if len(args) == 0 {
		fmt.Println("用法: network policy <add|list|remove> ...")
		return
	}

	switch args[0] {
	case "add":
		handlePolicyAdd(args[1:])
	case "list":
		policies := netMgr.ListPolicies()
		if len(policies) == 0 {
			fmt.Println("没有网络策略")
			return
		}
		fmt.Printf("%-15s %-15s %-20s %-20s\n", "NAME", "SANDBOX", "ALLOW_FROM", "ALLOW_TO")
		for _, p := range policies {
			fmt.Printf("%-15s %-15s %-20s %-20s\n",
				p.Name, p.Sandbox, fmt.Sprintf("%v", p.AllowFrom), fmt.Sprintf("%v", p.AllowTo))
		}
	case "remove":
		if len(args) < 2 {
			fmt.Println("用法: network policy remove <name>")
			return
		}
		netMgr.RemovePolicy(args[1])
		fmt.Printf("策略 %s 已移除\n", args[1])
	default:
		fmt.Printf("未知子命令: %s\n", args[0])
	}
}

func handlePolicyAdd(args []string) {
	var name, sandbox string
	var allowFrom, allowTo []string

	i := 0
	for i < len(args) {
		switch args[i] {
		case "--sandbox":
			if i+1 < len(args) {
				sandbox = args[i+1]
				i += 2
			} else {
				i++
			}
		case "--allow-from":
			if i+1 < len(args) {
				allowFrom = append(allowFrom, args[i+1])
				i += 2
			} else {
				i++
			}
		case "--allow-to":
			if i+1 < len(args) {
				allowTo = append(allowTo, args[i+1])
				i += 2
			} else {
				i++
			}
		default:
			if name == "" {
				name = args[i]
			}
			i++
		}
	}

	if name == "" || sandbox == "" {
		fmt.Println("用法: network policy add <name> --sandbox <name> [--allow-from <name>] [--allow-to <name>]")
		return
	}

	policy := &netpkg.NetworkPolicy{
		Name:      name,
		Sandbox:   sandbox,
		AllowFrom: allowFrom,
		AllowTo:   allowTo,
	}
	netMgr.AddPolicy(policy)
	fmt.Printf("策略 %s 已添加 (sandbox: %s, allow-from: %v)\n", name, sandbox, allowFrom)
}

func handleStorage(args []string) {
	if len(args) == 0 {
		fmt.Println(`storage 子命令:
  storage volume create <name> --size <bytes> --mode <RWO|ROX|RWX> --class <name>
  storage volume list
  storage volume inspect <name>
  storage volume delete <name>
  storage volume bind <name> --sandbox <name>
  storage volume unbind <name>
  storage layer create <name> [--parent <name>]
  storage fs create <sandbox> --layers <l1,l2,...>
  storage fs inspect <sandbox>
  storage fs delete <sandbox>
  storage snapshot create <name> --volume <name>
  storage snapshot list
  storage snapshot delete <name>
  storage class list`)
		return
	}

	switch args[0] {
	case "volume":
		handleStorageVolume(args[1:])
	case "layer":
		handleStorageLayer(args[1:])
	case "fs":
		handleStorageFS(args[1:])
	case "snapshot":
		handleStorageSnapshot(args[1:])
	case "class":
		classes := storageMgr.ListStorageClasses()
		fmt.Printf("%-15s %-10s %-15s\n", "NAME", "BACKEND", "PROVISIONER")
		for _, c := range classes {
			fmt.Printf("%-15s %-10s %-15s\n", c.Name, c.Backend, c.Provisioner)
		}
	default:
		fmt.Printf("未知子命令: %s\n", args[0])
	}
}

func handleStorageVolume(args []string) {
	if len(args) == 0 {
		fmt.Println("用法: storage volume <create|list|inspect|delete|bind|unbind> ...")
		return
	}

	switch args[0] {
	case "create":
		var name string
		var size int64
		var mode = stpkg.ReadWriteOnce
		var class string

		i := 0
		for i < len(args) {
			switch args[i] {
			case "--size":
				if i+1 < len(args) {
					size, _ = strconv.ParseInt(args[i+1], 10, 64)
					i += 2
				} else {
					i++
				}
			case "--mode":
				if i+1 < len(args) {
					mode = stpkg.AccessMode(args[i+1])
					i += 2
				} else {
					i++
				}
			case "--class":
				if i+1 < len(args) {
					class = args[i+1]
					i += 2
				} else {
					i++
				}
			default:
				if name == "" {
					name = args[i]
				}
				i++
			}
		}

		vol, err := storageMgr.CreateVolume(name, size, mode, class)
		if err != nil {
			fmt.Printf("错误: %v\n", err)
			return
		}
		fmt.Printf("卷 %s 已创建 (size: %d, mode: %s, class: %s)\n", vol.Name, vol.SizeBytes, vol.AccessMode, vol.StorageClass)

	case "list":
		vols := storageMgr.ListVolumes()
		if len(vols) == 0 {
			fmt.Println("没有卷")
			return
		}
		fmt.Printf("%-15s %-10s %-10s %-15s %-10s\n", "NAME", "SIZE", "MODE", "STATUS", "BOUND_TO")
		for _, v := range vols {
			fmt.Printf("%-15s %-10d %-10s %-15s %-10s\n", v.Name, v.SizeBytes, v.AccessMode, v.Status, v.BoundTo)
		}

	case "inspect":
		if len(args) < 2 {
			fmt.Println("用法: storage volume inspect <name>")
			return
		}
		vol, err := storageMgr.GetVolume(args[1])
		if err != nil {
			fmt.Printf("错误: %v\n", err)
			return
		}
		data, _ := json.MarshalIndent(vol, "", "  ")
		fmt.Println(string(data))

	case "delete":
		if len(args) < 2 {
			fmt.Println("用法: storage volume delete <name>")
			return
		}
		if err := storageMgr.DeleteVolume(args[1]); err != nil {
			fmt.Printf("错误: %v\n", err)
		} else {
			fmt.Printf("卷 %s 已删除\n", args[1])
		}

	case "bind":
		var volName, sandboxName string
		i := 0
		for i < len(args) {
			if args[i] == "--sandbox" && i+1 < len(args) {
				sandboxName = args[i+1]
				i += 2
			} else {
				if volName == "" {
					volName = args[i]
				}
				i++
			}
		}
		if err := storageMgr.BindVolume(volName, sandboxName); err != nil {
			fmt.Printf("错误: %v\n", err)
		} else {
			fmt.Printf("卷 %s 已绑定到 %s\n", volName, sandboxName)
		}

	case "unbind":
		if len(args) < 2 {
			fmt.Println("用法: storage volume unbind <name>")
			return
		}
		if err := storageMgr.UnbindVolume(args[1]); err != nil {
			fmt.Printf("错误: %v\n", err)
		} else {
			fmt.Printf("卷 %s 已解绑\n", args[1])
		}
	}
}

func handleStorageLayer(args []string) {
	if len(args) == 0 {
		fmt.Println("用法: storage layer create <name> [--parent <name>]")
		return
	}

	switch args[0] {
	case "create":
		var name, parent string
		i := 0
		for i < len(args) {
			if args[i] == "--parent" && i+1 < len(args) {
				parent = args[i+1]
				i += 2
			} else {
				if name == "" {
					name = args[i]
				}
				i++
			}
		}
		layer, err := storageMgr.CreateLayer(name, parent)
		if err != nil {
			fmt.Printf("错误: %v\n", err)
			return
		}
		fmt.Printf("层 %s 已创建 (parent: %s)\n", layer.Name, layer.Parent)
	}
}

func handleStorageFS(args []string) {
	if len(args) == 0 {
		fmt.Println("用法: storage fs <create|inspect|delete> ...")
		return
	}

	switch args[0] {
	case "create":
		var sandbox string
		var layersStr string
		i := 0
		for i < len(args) {
			if args[i] == "--layers" && i+1 < len(args) {
				layersStr = args[i+1]
				i += 2
			} else {
				if sandbox == "" {
					sandbox = args[i]
				}
				i++
			}
		}
		layers := splitComma(layersStr)
		fs, err := storageMgr.CreateFilesystem(sandbox, layers)
		if err != nil {
			fmt.Printf("错误: %v\n", err)
			return
		}
		fmt.Printf("文件系统已创建 (sandbox: %s, layers: %d)\n", fs.SandboxName, len(fs.LowerDirs))

	case "inspect":
		if len(args) < 2 {
			fmt.Println("用法: storage fs inspect <sandbox>")
			return
		}
		fs, err := storageMgr.GetFilesystem(args[1])
		if err != nil {
			fmt.Printf("错误: %v\n", err)
			return
		}
		data, _ := json.MarshalIndent(fs, "", "  ")
		fmt.Println(string(data))

	case "delete":
		if len(args) < 2 {
			fmt.Println("用法: storage fs delete <sandbox>")
			return
		}
		if err := storageMgr.DeleteFilesystem(args[1]); err != nil {
			fmt.Printf("错误: %v\n", err)
		} else {
			fmt.Printf("文件系统 %s 已删除\n", args[1])
		}
	}
}

func handleStorageSnapshot(args []string) {
	if len(args) == 0 {
		fmt.Println("用法: storage snapshot <create|list|delete> ...")
		return
	}

	switch args[0] {
	case "create":
		var name, volume string
		i := 0
		for i < len(args) {
			if args[i] == "--volume" && i+1 < len(args) {
				volume = args[i+1]
				i += 2
			} else {
				if name == "" {
					name = args[i]
				}
				i++
			}
		}
		snap, err := storageMgr.CreateSnapshot(name, volume)
		if err != nil {
			fmt.Printf("错误: %v\n", err)
			return
		}
		fmt.Printf("快照 %s 已创建 (volume: %s)\n", snap.Name, snap.VolumeName)

	case "list":
		snaps := storageMgr.ListSnapshots()
		if len(snaps) == 0 {
			fmt.Println("没有快照")
			return
		}
		fmt.Printf("%-15s %-15s %-10s\n", "NAME", "VOLUME", "SIZE")
		for _, s := range snaps {
			fmt.Printf("%-15s %-15s %-10d\n", s.Name, s.VolumeName, s.SizeBytes)
		}

	case "delete":
		if len(args) < 2 {
			fmt.Println("用法: storage snapshot delete <name>")
			return
		}
		if err := storageMgr.DeleteSnapshot(args[1]); err != nil {
			fmt.Printf("错误: %v\n", err)
		} else {
			fmt.Printf("快照 %s 已删除\n", args[1])
		}
	}
}

func splitComma(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}

func handleFaaS(args []string) {
	if len(args) == 0 {
		fmt.Println(`faas 子命令:
  faas register <name> --runtime <python|node|go> --handler <name> [--min-instances <n>] [--max-instances <n>]
  faas unregister <name>
  faas list
  faas inspect <name>
  faas invoke <name>
  faas instances
  faas warm-pool <name> --min-ready <n>
  faas reclaim`)
		return
	}

	switch args[0] {
	case "register":
		handleFaaSRegister(args[1:])
	case "unregister":
		if len(args) < 2 {
			fmt.Println("用法: faas unregister <name>")
			return
		}
		if err := faasEngine.Unregister(args[1]); err != nil {
			fmt.Printf("错误: %v\n", err)
		} else {
			fmt.Printf("函数 %s 已注销\n", args[1])
		}
	case "list":
		fns := faasEngine.ListFunctions()
		if len(fns) == 0 {
			fmt.Println("没有注册的函数")
			return
		}
		fmt.Printf("%-15s %-10s %-15s %-10s\n", "NAME", "RUNTIME", "HANDLER", "MIN_INST")
		for _, fn := range fns {
			fmt.Printf("%-15s %-10s %-15s %-10d\n", fn.Name, fn.Runtime, fn.Handler, fn.MinInstances)
		}
	case "inspect":
		if len(args) < 2 {
			fmt.Println("用法: faas inspect <name>")
			return
		}
		fn, err := faasEngine.GetFunction(args[1])
		if err != nil {
			fmt.Printf("错误: %v\n", err)
			return
		}
		data, _ := json.MarshalIndent(fn, "", "  ")
		fmt.Println(string(data))
	case "invoke":
		if len(args) < 2 {
			fmt.Println("用法: faas invoke <name>")
			return
		}
		result, err := faasEngine.Invoke(args[1], []byte("cli-invoke"))
		if err != nil {
			fmt.Printf("错误: %v\n", err)
			return
		}
		coldStartStr := "热启动"
		if result.ColdStart {
			coldStartStr = "冷启动"
		}
		fmt.Printf("结果: %s\n", string(result.Output))
		fmt.Printf("耗时: %v (%s)\n", result.Duration, coldStartStr)
	case "instances":
		insts := faasEngine.ListInstances()
		if len(insts) == 0 {
			fmt.Println("没有实例")
			return
		}
		fmt.Printf("%-10s %-15s %-10s %-10s\n", "ID", "FUNCTION", "STATUS", "REQS")
		for _, inst := range insts {
			fmt.Printf("%-10s %-15s %-10s %-10d\n", inst.ID, inst.FunctionName, inst.Status, inst.RequestCount)
		}
	case "warm-pool":
		handleFaaSWarmPool(args[1:])
	case "reclaim":
		reclaimed := faasEngine.ReclaimIdle()
		fmt.Printf("回收了 %d 个空闲实例\n", reclaimed)
	default:
		fmt.Printf("未知子命令: %s\n", args[0])
	}
}

func handleFaaSRegister(args []string) {
	var name, runtimeName, handler string
	var minInst, maxInst int

	i := 0
	for i < len(args) {
		switch args[i] {
		case "--runtime":
			if i+1 < len(args) {
				runtimeName = args[i+1]
				i += 2
			} else {
				i++
			}
		case "--handler":
			if i+1 < len(args) {
				handler = args[i+1]
				i += 2
			} else {
				i++
			}
		case "--min-instances":
			if i+1 < len(args) {
				minInst, _ = strconv.Atoi(args[i+1])
				i += 2
			} else {
				i++
			}
		case "--max-instances":
			if i+1 < len(args) {
				maxInst, _ = strconv.Atoi(args[i+1])
				i += 2
			} else {
				i++
			}
		default:
			if name == "" {
				name = args[i]
			}
			i++
		}
	}

	if name == "" {
		fmt.Println("用法: faas register <name> --runtime <python|node|go> --handler <name> [--min-instances <n>] [--max-instances <n>]")
		return
	}

	fn := &faaspkg.Function{
		Name:         name,
		Runtime:      runtimeName,
		Handler:      handler,
		Code:         "/code/" + name,
		Timeout:      30,
		MemoryLimit:  536870912,
		MinInstances: minInst,
		MaxInstances: maxInst,
	}

	if err := faasEngine.Register(fn); err != nil {
		fmt.Printf("错误: %v\n", err)
		return
	}

	fmt.Printf("函数 %s 已注册 (runtime: %s, handler: %s, min-instances: %d)\n",
		name, runtimeName, handler, minInst)
}

func handleFaaSWarmPool(args []string) {
	var funcName string
	var minReady int

	i := 0
	for i < len(args) {
		switch args[i] {
		case "--min-ready":
			if i+1 < len(args) {
				minReady, _ = strconv.Atoi(args[i+1])
				i += 2
			} else {
				i++
			}
		default:
			if funcName == "" {
				funcName = args[i]
			}
			i++
		}
	}

	if funcName == "" {
		fmt.Println("用法: faas warm-pool <name> --min-ready <n>")
		return
	}

	faasEngine.SetWarmPoolConfig(funcName, faaspkg.WarmPoolConfig{
		MinReady:       minReady,
		MaxIdle:        5 * 1000000000 * 60, // 5 minutes
		ReplenishDelay: 100 * 1000000,       // 100ms
	})

	ready, total := faasEngine.GetWarmPoolStatus(funcName)
	fmt.Printf("预热池: %d 就绪 / %d 总计 (min-ready: %d)\n", ready, total, minReady)
}

func handleAPI(args []string) {
	if len(args) == 0 {
		fmt.Println(`api 子命令:
  api server start [--addr <host:port>]   启动 API Server
  api resource create <kind> <name>        创建资源（声明式）
  api resource list <kind>                 列出资源
  api resource get <kind> <name>           查询资源
  api resource update <kind> <name>        更新资源 Spec
  api resource delete <kind> <name>        删除资源
  api controller list                      列出已注册的控制器
  api controller run                       启动控制器循环`)
		return
	}

	switch args[0] {
	case "server":
		handleAPIServer(args[1:])
	case "resource":
		handleAPIResource(args[1:])
	case "controller":
		handleAPIController(args[1:])
	default:
		fmt.Printf("未知子命令: %s\n", args[0])
	}
}

func handleAPIServer(args []string) {
	addr := ":8080"
	for i := 0; i < len(args); i++ {
		if args[i] == "--addr" && i+1 < len(args) {
			addr = args[i+1]
		}
	}

	fmt.Printf("API Server 启动在 %s\n", addr)
	fmt.Println("可用端点:")
	fmt.Println("  GET    /api/v1/sandboxes           列出沙箱")
	fmt.Println("  POST   /api/v1/sandboxes           创建沙箱")
	fmt.Println("  GET    /api/v1/sandboxes/<name>    查询沙箱")
	fmt.Println("  PUT    /api/v1/sandboxes/<name>    更新沙箱")
	fmt.Println("  DELETE /api/v1/sandboxes/<name>    删除沙箱")
	fmt.Println("  GET    /api/v1/sandboxes?watch=true Watch 沙箱")
	fmt.Println("  (同样模式适用于 nodes, volumes, functions, policies)")
	fmt.Println()
	fmt.Println("按 Ctrl+C 停止")

	if err := apiServer.Start(addr); err != nil {
		fmt.Printf("API Server 错误: %v\n", err)
	}
}

func handleAPIResource(args []string) {
	if len(args) < 2 {
		fmt.Println("用法: api resource <create|list|get|update|delete> <kind> [name]")
		return
	}

	switch args[0] {
	case "create":
		if len(args) < 3 {
			fmt.Println("用法: api resource create <kind> <name>")
			return
		}
		obj, err := apiStore.Create(args[1], args[2], map[string]interface{}{"created": "via-cli"})
		if err != nil {
			fmt.Printf("错误: %v\n", err)
			return
		}
		fmt.Printf("资源 %s/%s 已创建 (version: %d)\n", args[1], args[2], obj.Meta.ResourceVersion)

	case "list":
		list := apiStore.List(args[1])
		if len(list) == 0 {
			fmt.Printf("没有 %s 资源\n", args[1])
			return
		}
		fmt.Printf("%-15s %-15s %-10s\n", "KIND", "NAME", "VERSION")
		for _, obj := range list {
			fmt.Printf("%-15s %-15s %-10d\n", args[1], obj.Meta.Name, obj.Meta.ResourceVersion)
		}

	case "get":
		if len(args) < 3 {
			fmt.Println("用法: api resource get <kind> <name>")
			return
		}
		obj, err := apiStore.Get(args[1], args[2])
		if err != nil {
			fmt.Printf("错误: %v\n", err)
			return
		}
		data, _ := json.MarshalIndent(obj, "", "  ")
		fmt.Println(string(data))

	case "update":
		if len(args) < 3 {
			fmt.Println("用法: api resource update <kind> <name>")
			return
		}
		obj, err := apiStore.Update(args[1], args[2], map[string]interface{}{"updated": "via-cli"})
		if err != nil {
			fmt.Printf("错误: %v\n", err)
			return
		}
		fmt.Printf("资源 %s/%s 已更新 (version: %d)\n", args[1], args[2], obj.Meta.ResourceVersion)

	case "delete":
		if len(args) < 3 {
			fmt.Println("用法: api resource delete <kind> <name>")
			return
		}
		if err := apiStore.Delete(args[1], args[2]); err != nil {
			fmt.Printf("错误: %v\n", err)
		} else {
			fmt.Printf("资源 %s/%s 已删除\n", args[1], args[2])
		}
	}
}

func handleAPIController(args []string) {
	if len(args) == 0 {
		fmt.Println("用法: api controller <list|run>")
		return
	}

	switch args[0] {
	case "list":
		fmt.Println("已注册的控制器:")
		fmt.Println("  (控制器通过代码注册，CLI 暂不支持动态注册)")

	case "run":
		fmt.Println("控制器循环已启动（后台运行）")
		ctrlMgr.Run()
		fmt.Println("使用 api resource create/update/delete 来触发 Reconcile")
		fmt.Println("按 Ctrl+C 停止")
		select {}
	}
}

func printUsage() {
	fmt.Println(`用法: cloudruntime <command>

可用命令:
  version    显示版本信息
  sandbox    隔离与沙箱管理 (模块 1)
  scheduler  调度与编排 (模块 2)
  network    网络抽象 (模块 3)
  storage    存储抽象 (模块 4)
  faas       函数运行时 (模块 5)
  api        控制面 API (模块 6)
  help       显示帮助

学习路径: 模块 0(概览) → 1(隔离) → 2(调度) → 3(网络) → 4(存储) → 5(FaaS) → 6(API)`)
}
