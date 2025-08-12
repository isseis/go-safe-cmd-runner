# Requirements for Logging System Redesign

## 1. Overview

This document outlines the requirements for redesigning the logging system of the `runner` command. The goal is to improve the reliability, usability, and extensibility of the logging functionality.

## 2. Background

The current logging implementation in the `runner` command has the following issues:
- The specified log level via the `--log-level` flag is not consistently respected because of the mixed use of the standard `log` package and the `log/slog` package.
- It lacks the ability to output logs in multiple formats simultaneously (e.g., machine-readable for analysis and human-readable for quick summaries).

## 3. Functional Requirements

### FR1: Strict Log Level Filtering
The system must strictly filter log messages based on the log level specified by the user. Messages with a severity lower than the specified level must not be output.

### FR2: Dual Logging Output
The system must support outputting logs to two different destinations with different formats and log levels simultaneously.
- **Machine-Readable Log**: A detailed, structured log (e.g., JSON format) for auditing and debugging purposes.
- **Human-Readable Summary**: A concise summary of the execution results for quick user feedback (e.g., posting to Slack).

### FR3: Centralized Logging Interface
All logging in the application must be performed through a single, unified logging interface, which will be `log/slog`.

### FR4: Configurable Log File Path
The path for the machine-readable log file must be configurable.

### FR5: Default to Standard Output
If no specific configuration is provided for the human-readable summary, it should be output to the standard output by default.

## 4. Non-Functional Requirements

### NFR1: Performance
The logging system should have minimal performance impact on the main application logic.

### NFR2: Extensibility
The design should be extensible to support other output formats or destinations in the future (e.g., sending logs to a remote server, different notification systems).

### NFR3: Ease of Use
For developers, using the logging system should be straightforward. The complexity of handling multiple outputs should be abstracted away from the application code.
