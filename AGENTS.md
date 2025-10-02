# Repository Guidelines

## General

### Agents

Multiple agents might be modifying the code at the same time. If you come across changes that aren't yours, preserve them and avoid interfering with other agents' work. Only halt the task and inform me when you encounter an irreconcilable conflict.

### TimeZone

Always use UTC for time handling in servers, databases, and APIs.

### Date Range

For any date‑range query, the handling of the ending date must encompass the entire final day. That means the database query should terminate **just before** 00:00 on the next day, ensuring that all hours of the last day are included.

### Testing

Please create suitable unit tests based on the current project circumstances. Whenever a new issue arises, update the unit tests during the fix to ensure thorough coverage of the problem by the test cases. Avoid creating temporary, one-off test scripts, and focus on continuously enhancing the unit test cases.

## Golang Style

### Context

Whenever feasible, utilize context to manage the lifecycle of the call chain.

### Golang Error Handling

All errors should be handled, and the error handling should be as close to the source of the error as possible.

Never use `err == nil` to avoid shadowing the error variable.

Use `github.com/Laisky/errors/v2`, its interface is as same as `github.com/Laisky/errors/v2`. Never return bare error, always wrap it by `errors.Wrap`/`errors.Wrapf`/`errors.WithStack`, check all files

Every error must be processed a single time—either returned or logged—but never both.

Avoid returning raw errors; wrap them with errors.Wrap, errors.Wrapf, or errors.WithStack to preserve essential stack traces and contextual information.

### Golang ORM

Use `gorm.io/gorm`, never use `gorm.io/gorm/clause`/`Preload`.

The performance of ORMs is often quite inefficient. Therefore, adopt the data reading method that puts the least pressure on the database whenever possible. my philosophy is to use SQL for reading and reserve ORM for writing or modifying data.

Example:

```go
// When retrieving data, utilize Model/Find/First as much as possible,
// and rely on SQL for query conditions whenever you can.
db.Model(&User{}).
    Joins("JOIN emails ON emails.user_id = users.id AND emails.email = ?", "jinzhu@example.org").
    Joins("JOIN credit_cards ON credit_cards.user_id = users.id").Where("credit_cards.number = ?", "411111111111").
    Find(&user)

// Use Scan only when the data being read does not align with the database table structure.
db.Model(&User{}).
    Select("users.name AS name, emails.email AS email").
    Joins("left join emails on emails.user_id = users.id").
    Scan(&result{})

```

### Logging

All code paths invoked by a request must use `gmw.GetLogger(c)` to retrieve the logger instead of the global `logger.Logger`. The logger returned by `gmw.GetLogger(c)` embeds rich call‑specific context.

Adopt these logger and error‑handling best practices:

1. Call `gmw.GetLogger(c)` only once per function and store the result in a local variable.
2. Use `zap.Error(err)` rather than `err.Error()` when logging errors.
3. Prefer the structured Zap logger over `fmt.Sprintf` for log messages.
4. Never swallow errors silently; every error should be returned or recorded in the logs.

## CSS Style

Avoid using `!important` in CSS. If you find yourself needing to use it, consider whether the CSS can be refactored to avoid this necessity.

Avoid inline styles in HTML or JSX. Instead, use CSS classes to manage styles. This approach promotes better maintainability and separation of concerns in your codebase.

## Web

When using the web console for debugging, avoid logging objects—they’re hard to copy. Strive to log only strings, making it simple for me to copy all the output and analyze it.
