# go-scheduler Project Instructions

## Project Overview

This is a reusable Go job scheduling framework library that provides comprehensive job scheduling capabilities including cron scheduling, manual execution, scheduled execution (XXL-JOB style), compensation mechanism, and configuration management.

## Project Structure

-   `scheduler/` - Core scheduler implementation
-   `job/` - Job interfaces and base implementations
-   `repository/` - Data persistence layer (memory, database, cache)
-   `models/` - Data models and converters
-   `pubsub/` - Event publishing and subscription
-   `logger/` - Logger adapter interface
-   `internal/` - Internal utilities
-   `examples/` - Usage examples

## Dependencies

-   github.com/kamalyes/go-cachex - Cache operations
-   github.com/kamalyes/go-logger - Logging
-   github.com/kamalyes/go-sqlbuilder - Database operations
-   github.com/kamalyes/go-scheduler - Utilities
-   github.com/robfig/cron/v3 - Cron scheduling
-   gorm.io/gorm - ORM

## Development Status

Project creation in progress.
