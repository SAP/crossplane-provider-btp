/*
This example demonstrates a CLI implementation with the `export`
subcommand that generates test resource definitions when invoked.

Example:

   go run ./main.go export

This will print 20 test resources to the console. This example also
demonstrates the warning-level log message generation.

After every 5 resource, a warning message is printed to the screen.

It is also possible to write the generated YAML object to a file:

   go run ./main.go export -o output.yaml

The log messages are printed to the screen but the YAML documents are
written to the file `output.yaml`.

The export subcommand has a mandatory '--kind'/'-k' flag. You can see
it in action with the following command:

   go run ./main.go export -v -o /dev/null --kind resource1 --kind resource2
*/
package main
