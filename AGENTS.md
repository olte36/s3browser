# AGENTS.md

## Code Style

Go files should be organized in this order:

1. Constants
2. Variables
3. Interfaces
4. Structures
5. `NewXXX` functions
6. Public methods
7. Private methods
8. Public functions
9. Private functions

Always write descriptive comments that reflect the purpose of a struct, function, method, interface, constant, variable, or other declaration.

## Testing

- Use the Go standard library for testing.
- Locate all auxiliary functions and methods at the end of a test file.
