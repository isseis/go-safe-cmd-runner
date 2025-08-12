# Architecture for Logging System Redesign

## 1. Overview

This document describes the proposed architecture for the redesigned logging system. It is based on the requirements outlined in `01_requirements.md`. The core of this architecture is to fully leverage the `log/slog` package and its `slog.Handler` interface to create a flexible and powerful logging pipeline.

## 2. Architectural Principles

- **Single Responsibility**: Each component in the logging system should have a single, well-defined responsibility.
- **Decoupling**: The application code that generates logs should be decoupled from the specifics of how logs are formatted and where they are sent.
- **Extensibility**: The architecture should be easy to extend with new logging handlers without modifying the core application logic.

## 3. Proposed Architecture

The proposed architecture consists of three main components:
1.  **Unified Logger**: A single `slog.Logger` instance used throughout the application.
2.  **Multi-Handler**: A custom `slog.Handler` that dispatches log records to multiple underlying handlers.
3.  **Specific Handlers**: Individual `slog.Handler` implementations for each output destination (machine-readable file and human-readable summary).

### System Diagram

```
+---------------------+      +-------------------+      +------------------------+
| Application Code    |----->|   slog.Logger     |----->|      MultiHandler      |
| (e.g., runner)      |      | (Default Logger)  |      | (Custom slog.Handler)  |
+---------------------+      +-------------------+      +------------------------+
                                                            |
                                                            |
                                     +----------------------+----------------------+
                                     |                                             |
                                     v                                             v
                         +-------------------------+                   +-------------------------+
                         |   JSON Handler          |                   |   Text Handler          |
                         | (slog.NewJSONHandler)   |                   | (slog.NewTextHandler)   |
                         +-------------------------+                   +-------------------------+
                                     |                                             |
                                     v                                             v
                         +-------------------------+                   +-------------------------+
                         | Machine-Readable Log    |                   | Human-Readable Summary  |
                         | (runner-log.json)       |                   | (stdout / Slack)        |
                         +-------------------------+                   +-------------------------+
```

### Component Descriptions

#### 3.1. Application Code
- All parts of the `runner` application will use the standard `slog` functions (e.g., `slog.Debug`, `slog.Info`, `slog.Error`).
- The code will no longer use the standard `log` package. This ensures all log messages go through the `slog` pipeline.

#### 3.2. `slog.Logger`
- A single, global logger instance will be configured at the application's entry point (`main` function).
- This logger will be initialized with the `MultiHandler`.

#### 3.3. `MultiHandler`
- This is a custom implementation of the `slog.Handler` interface.
- Its primary role is to hold a list of other `slog.Handler` instances.
- When its `Handle` method is called, it iterates through its list of handlers and calls the `Handle` method on each one.
- It will also be responsible for checking if a handler is enabled for a given log level before dispatching the record.

#### 3.4. JSON Handler
- An instance of `slog.NewJSONHandler`.
- **Responsibility**: To format log records as JSON.
- **Output**: A specified log file (e.g., `runner-log.json`).
- **Log Level**: Configured based on the `--log-level` command-line flag. This allows for detailed logs for debugging.

#### 3.5. Text Handler
- An instance of `slog.NewTextHandler`.
- **Responsibility**: To format log records in a human-readable, plain-text format.
- **Output**: Standard output by default. This output can be piped to other tools or a future handler could send it to a service like Slack.
- **Log Level**: This will be fixed to a higher level, such as `slog.LevelInfo`. This ensures that only important summary messages are displayed, regardless of the `--log-level` setting for the JSON log.

## 4. Initialization Flow

1.  **Parse Flags**: The `main` function will parse command-line flags, including `--log-level` and a new `--log-file` flag.
2.  **Open Log File**: Open the file specified by `--log-file`.
3.  **Create Handlers**:
    - Instantiate `JSONHandler` with the file writer and the parsed log level.
    - Instantiate `TextHandler` with `os.Stdout` and a fixed `slog.LevelInfo`.
4.  **Create MultiHandler**: Instantiate the custom `MultiHandler` with the `JSONHandler` and `TextHandler`.
5.  **Create Logger**: Create a new `slog.Logger` with the `MultiHandler`.
6.  **Set Default Logger**: Call `slog.SetDefault` to make this logger the global default for the application.

## 5. Extensibility
To add a new logging output (e.g., Slack), a new `SlackHandler` would be created and added to the `MultiHandler` during initialization, with no changes required to the rest of the application.
