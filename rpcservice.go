package taskmaster

//go:generate go run ./cmd/rpc_method_const/ -type RPCService

// RPCService will be the type on which we define our RPC methods
type RPCService struct {
	service *Service
}

// NewRPCService creates a new RPCService with a reference to the Service
func NewRPCService(service *Service) *RPCService {
	return &RPCService{service: service}
}

// List retrieves a list of names and sets it to the provided names pointer.
//
// Parameters:
//   - _: An empty struct, as this method does not require any input parameters.
//   - names: A pointer to a slice of strings where the list of names will be stored.
//
// Returns:
//   - An error if the retrieval fails.
func (r *RPCService) List(_ struct{}, names *[]string) error {
	*names = r.service.List()
	return nil
}

// Get retrieves a command by name and sets it's pid to the provided pointer.
//
// Parameters:
//   - name: The name of the command to retrieve.
//   - pid: A pointer to an int where the retrieved command pid will be stored.
//
// Returns:
//   - An error if the retrieval fails.
func (r *RPCService) Status(name string, pid *int) error {
	cmd, err := r.service.Get(name)
	if err != nil {
		return err
	}

	*pid = cmd.Process.Pid

	return nil
}

// Start starts a service by name.
//
// Parameters:
//   - name: The name of the service to start.
//
// Returns:
//   - An error if the start operation fails.
func (r *RPCService) Start(name string, _ *struct{}) error {
	return r.service.Start(name)
}

// Stop stops a service by name.
//
// Parameters:
//   - name: The name of the service to stop.
//
// Returns:
//   - An error if the stop operation fails.
func (r *RPCService) Stop(name string, _ *struct{}) error {
	return r.service.Stop(name)
}

// ReloadConfig reloads the configuration of the service.
//
// Parameters:
//   - _: An empty struct, as this method does not require any input parameters.
//   - reply: A pointer to a boolean where the result of the reload operation will be stored.
//
// Returns:
//   - An error if the reload operation fails.
func (r *RPCService) ReloadConfig(_ struct{}, reply *bool) error {
	changed, err := r.service.ReloadConfig()
	*reply = changed
	return err
}
