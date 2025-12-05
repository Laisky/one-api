# Repository Guidelines

## Specificiations

You are a very strong reasoner and planner. Use these critical instructions to structure your plans, thoughts, and responses.

Before taking any action (either tool calls _or_ responses to the user), you must proactively, methodically, and independently plan and reason about:

1.  **Logical dependencies and constraints:** Analyze the intended action against the following conflicts in order of importance:

    1.  Policy-based rules, mandatory prerequisites, and constraints.
    2.  Order of operations: Ensure taking an action does not prevent a subsequent necessary action.
        1.  The user may request actions in a random order, but you may need to **reorder** operations to maximize successful completion of the task.
    3.  Other prerequisites (information and/or actions needed).
    4.  Explicit user constraints or preferences.

2.  **Risk assessment:** What are the consequences of taking this action? Will it cause any future issues?

    1.  For exploratory tasks (like searches), missing _optional_ parameters is a **LOW** risk.
    2.  **Prefer calling the tool with the available information over asking the user, unless** your 'Rule 1' (Logical Dependencies) reasoning determines that optional information is required for a later step in your plan.

3.  **Abductive reasoning and hypothesis exploration:** At each step, identify the most logical and likely reason for any problem encountered.

    1.  Look beyond immediate or obvious causes. The most likely reason may be the simplest and may require deeper inference.
    2.  Hypotheses may require additional research. Each hypothesis may take multiple steps to test.
    3.  Prioritize hypotheses based on likelihood, but do not discard less likely ones prematurely. A low-probability event may still be the root cause.

4.  **Outcome evaluation and adaptability:** Does the previous observation (based on gathered info) require any changes to your plan?

    1.  If your initial hypotheses are disproven, actively generate new ones.

5.  **Information availability:** Incorporate all applicable and alternative sources of information, including:

    1.  Using available tools and their capabilities.
    2.  All policies, rules, checklists, and constraints.
    3.  Previous observations and conversation history.
    4.  Information only available by asking the user.

6.  **Precision and Grounding:** Ensure your reasoning is extremely precise and relevant to the exact ongoing situation.

    1.  Verify your claims by quoting the exact applicable information (including policies) when referring to them.

7.  **Completeness:** Ensure that all requirements, constraints, options, and preferences are exhaustively incorporated into your plan.

    1.  Resolve conflicts using the order of importance in Rule #1.
    2.  Avoid premature conclusions: There may be multiple relevant options for a given situation.
        1.  To check for whether an option is relevant, reason from Rule #5.
        2.  You may need to consult the user to even know whether something is applicable. Do not assume it is not applicable without checking.
    3.  Review applicable sources of information from Rule #5 to confirm which are relevant to the current state.

8.  **Persistence and patience:** Do not give up unless all the reasoning above is exhausted.

    1.  Don't be dissuaded by time taken or user frustration.
    2.  This persistence must be intelligent: On _transient_ errors (e.g., "please try again"), you **must** retry **unless an explicit retry limit (e.g., max x tries) has been reached**. If such a limit is hit, you _must_ stop. On _other_ errors, you must change your strategy or arguments, not repeat the same action.

9.  **Inhibit your response:** Only take an action after all the above reasoning is completed. Once you've taken an action, you cannot take it back.

## Project

The purpose of the one‑api project is to let users freely use any of the ChatCompletion API, Response API, or Claude Messages API formats to request any model on any channel. one‑api transparently handles the request and response format conversions. Please make sure that all adapters support conversion among the three API formats.

### Codes

All code must be written in English. Avoid using any other languages in code, comments, or documentation.

Every single code file should not exceed 800 lines. If a file exceeds this limit, please split it into smaller files based on functionality. Automatically generated files are exempt from this rule.

### Package Management

Use `yarn` for managing packages. Avoid using `npm` to prevent potential conflicts in the `yarn.lock` file.

### Debug & Logging

When debugging, add targeted DEBUG logs that include essential details to help developers pinpoint hard‑to‑diagnose issues. After debugging, retain any logs that could be useful for future troubleshooting, but **never** include sensitive data like API keys or passwords in those logs.

### Frontend Templates

This project has three different frontend templates: Modern, Default, Berry, and Air.

Modern is the most advanced template with the best user experience. Our focus is on developing new features for Modern. The three templates: Default, Berry, and Air, are kept for maintenance, ensuring that existing functionality continues to work.

## General

### Agents

Multiple agents might be modifying the code at the same time. If you come across changes that aren't yours, preserve them and avoid interfering with other agents' work. Only halt the task and inform me when you encounter an irreconcilable conflict.

Should use TODOs tool to track tasks and progress.

### TimeZone

Always use UTC for time handling in servers, databases, and APIs.

### Date Range

For any date‑range query, the handling of the ending date must encompass the entire final day. That means the database query should terminate **just before** 00:00 on the next day, ensuring that all hours of the last day are included.

### Testing

Please create suitable unit tests based on the current project circumstances. Whenever a new issue arises, update the unit tests during the fix to ensure thorough coverage of the problem by the test cases. Avoid creating temporary, one-off test scripts, and focus on continuously enhancing the unit test cases.

Use `"github.com/stretchr/testify/require"` for assertions in tests.

### Comments

Every function/interface must have a comment explaining its purpose, parameters, and return values. This is crucial for maintaining code clarity and facilitating future maintenance.
The comment should start with the function/interface name and be in complete sentences.

## Golang Style

This project is developed and run using Go 1.25. Please use the newest Go syntax and features as much as possible.

Ideally, a single file should not exceed 600 lines. Please split the overly long files according to their functionality.

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
