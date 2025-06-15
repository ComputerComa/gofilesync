# Current Issue

## Description
The first few lines of the log file are not using the custom `[DEBUG] TIME - MESSAGE` format. Instead, they are logged in the default JSON format. This issue persists even after initializing the custom logger early in the `main` function and attempting to replace the global logger with the custom logger.

## Steps Taken
1. Updated the `InitLogger` function to use a custom encoder configuration for consistent formatting.
2. Ensured the logger is initialized as early as possible in the `main` function.
3. Attempted to replace the global logger with the custom logger using `zap.ReplaceGlobals`.

## Next Steps
- Investigate why the default JSON format is still being used for the first few log entries.
- Ensure the custom logger configuration is applied globally before any logging occurs.
- Test the changes to confirm consistent log formatting from the start.

## Priority
High
