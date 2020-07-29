// +build linux

package main

import (
	"bytes"
	"flag"
	"fmt"
	"github.com/docker/docker/pkg/reexec"
	"os"
	"os/exec"
	"syscall"

	"path/filepath"
	"net"
	"time"

)

func init() {

	fmt.Printf("arg0=%s,\n",os.Args[0])

	reexec.Register("initFuncName", func() {
		fmt.Printf("\n>> namespace setup code goes here <<\n\n")

		newRoot := os.Args[1]

		if err := mountProc(newRoot); err != nil {
			fmt.Printf("Error mounting /proc - %s\n", err)
			os.Exit(1)
		}

		fmt.Printf("newRoot:%s \n",newRoot)
		if err := pivotRoot(newRoot); err != nil {
			fmt.Printf("Error running pivot_root - %s\n", err)
			os.Exit(1)
		}


		if err := waitNetwork(); err != nil {
			fmt.Printf("Error waiting for network - %s\n", err)
			os.Exit(1)
		}


		nsRun() //calling clone() to create new process goes here
	})

	if reexec.Init() {
		os.Exit(0)
	}
}


func nsRun() {
	cmd := exec.Command("sh")

    cmd.Env = []string{"PATH=/sbin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/bin"}
	//set identify for this demo
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr


	if err := cmd.Run(); err != nil {
		fmt.Printf("Error running the /bin/sh command - %s\n", err)
		os.Exit(1)
	}
}

func main() {

	var rootfsPath,netsetPath string

	flag.StringVar(&netsetPath, "netsetPath", "./netsetter.sh", "Path to the netset shell")
	flag.StringVar(&rootfsPath, "rootfs", "/tmp/ns-proc/rootfs", "Path to the root filesystem to use")
	flag.Parse()

	checkRootfs(rootfsPath)
	checkNetsetter(netsetPath)

	cmd := reexec.Command("initFuncName",rootfsPath)

    cmd.Env = []string{"PATH=/sbin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/bin"}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWNS |
			syscall.CLONE_NEWUTS |
			syscall.CLONE_NEWIPC |
			syscall.CLONE_NEWPID |
			syscall.CLONE_NEWNET |
			syscall.CLONE_NEWUSER,
		UidMappings: []syscall.SysProcIDMap{
			{
				ContainerID: 0,
				HostID:      os.Getuid(),
				Size:        1,
			},
		},
		GidMappings: []syscall.SysProcIDMap{
			{
				ContainerID: 0,
				HostID:      os.Getgid(),
				Size:        1,
			},
		},
	}



	if err := cmd.Start(); err != nil {
		fmt.Printf("Error starting the reexec.Command - %s\n", err)
		os.Exit(1)
	}

	// run netsetgo using default args

	pid := fmt.Sprintf("%d", cmd.Process.Pid)

	//netsetCmd := exec.Command("whoami" ) //see current user , my result is ubuntu not root
	netsetCmd := exec.Command("sudo",netsetPath, pid) //
	var out bytes.Buffer
	var stderr bytes.Buffer
	netsetCmd.Stdout = &out
	netsetCmd.Stderr = &stderr

	if err := netsetCmd.Start(); err != nil {
		fmt.Printf("Error running netsetg:%s, stderr:%s, stdout:%s",fmt.Sprint(err),stderr.String(),out.String())
		os.Exit(1)
	}
	fmt.Printf("run netsetter: stdout:%s \n",out.String())

	if err := cmd.Wait(); err != nil {
		fmt.Printf("Error waiting for the reexec.Command - %s\n", err)
		os.Exit(1)
	}

}



func checkNetsetter(netsetPath string) {
	if _, err := os.Stat(netsetPath); os.IsNotExist(err) {
		errMsg := fmt.Sprintf(`file %s not found! you must have a netsetter binary or shell and run with root privilege，
or run with argument -netsetPath path_to_your_netsetter
`, netsetPath)
		fmt.Println(errMsg)
		os.Exit(1)
	}
}


func waitNetwork() error {
	maxWait := time.Second * 60
	checkInterval := time.Second
	timeStarted := time.Now()

	for {
		fmt.Printf("status: waiting network ...\n")
		interfaces, err := net.Interfaces()
		if err != nil {
			return err
		}

		if len(interfaces) > 1 {
			return nil
		}

		if time.Since(timeStarted) > maxWait {
			return fmt.Errorf("Timeout after %s waiting for network", maxWait)
		}

		time.Sleep(checkInterval)
	}
}


func checkRootfs(rootfsPath string) {
	if _, err := os.Stat(rootfsPath); os.IsNotExist(err) {
		fmt.Printf("rootfsPath %s is not found you may need to download it",rootfsPath)
		os.Exit(1)
	}
}

//implement pivot_root by syscall
func pivotRoot(newroot string) error {

	preRoot := "/.pivot_root"
	putold := filepath.Join(newroot,preRoot) //putold:/tmp/ns-proc/rootfs/.pivot_root


	// pivot_root requirement that newroot and putold must not be on the same filesystem as the current root
	//current root is / and new root is /tmp/ns-proc/rootfs and putold is /tmp/ns-proc/rootfs/.pivot_root
	//thus we bind mount newroot to itself to make it different
	//try to comment here you can see the error
	if err := syscall.Mount(newroot, newroot, "", syscall.MS_BIND|syscall.MS_REC, ""); err != nil {
		fmt.Printf("mount newroot:%s to itself error \n",newroot)
		return err
	}

	// create putold directory, equal to mkdir -p xxx
	if err := os.MkdirAll(putold, 0700); err != nil {
		fmt.Printf("create putold directory %s erro \n",putold)
		return err
	}

	// call pivot_root
	if err := syscall.PivotRoot(newroot, putold); err != nil {
		fmt.Printf("call PivotRoot error, newroot:%s,putold:%s \n",newroot,putold)
		return err
	}

	// ensure current working directory is set to new root
	if err := os.Chdir("/"); err != nil {
		return err
	}

	// umount putold, which now lives at /.pivot_root
	putold = preRoot
	if err := syscall.Unmount(putold, syscall.MNT_DETACH); err != nil {
		fmt.Printf("umount putold:%s error \n",putold)
		return err
	}

	// remove putold
	if err := os.RemoveAll(putold); err != nil {
		fmt.Printf("remove putold:%s error \n",putold)
		return err
	}

	return nil
}


func mountProc(newroot string) error {
	source := "proc"
	target := filepath.Join(newroot, "/proc")
	fstype := "proc"
	flags := 0
	data := ""

	os.MkdirAll(target, 0755)
	if err := syscall.Mount(
		source,
		target,
		fstype,
		uintptr(flags),
		data,
	); err != nil {
		return err
	}

	return nil
}






