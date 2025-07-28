package main

import (
	"fmt"
	"net/rpc"
	"os"
	"syscall"

	"github.com/souhoc/taskmaster"
)

func main() {
	client, err := rpc.Dial("unix", taskmaster.SocketName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error dialing socket: %v", err)
		os.Exit(1)
	}
	defer client.Close()

	var rep int
	if err := client.Call("TaskMaster.Pid", &struct{}{}, &rep); err != nil {
		fmt.Println("Error calling:", err)
		os.Exit(1)
	}
	fmt.Printf("pid: %d\n", rep)
	syscall.Kill(rep, syscall.SIGINT)
}
