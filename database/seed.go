package database

import (
	"database/sql"
	"log"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"github.com/russross/blackfriday/v2"
)

func Seed(db *sql.DB) error {
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM posts").Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		log.Println("Database already seeded, skipping")
		return nil
	}

	log.Println("Seeding database with sample data...")

	// Insert categories
	categories := map[string]string{
		"Go":           "go",
		"DevOps":       "devops",
		"Architecture": "architecture",
		"JavaScript":   "javascript",
		"Tools":        "tools",
	}
	for name, slug := range categories {
		if _, err := db.Exec("INSERT OR IGNORE INTO categories (name, slug) VALUES (?, ?)", name, slug); err != nil {
			return err
		}
	}

	// Insert tags
	tags := map[string]string{
		"golang":      "golang",
		"gin":         "gin",
		"api":         "api",
		"concurrency": "concurrency",
		"goroutines":  "goroutines",
		"sqlite":      "sqlite",
		"database":    "database",
		"docker":      "docker",
		"devops":      "devops",
		"typescript":  "typescript",
		"javascript":  "javascript",
		"react":       "react",
	}
	for name, slug := range tags {
		if _, err := db.Exec("INSERT OR IGNORE INTO tags (name, slug) VALUES (?, ?)", name, slug); err != nil {
			return err
		}
	}

	policy := bluemonday.UGCPolicy()

	posts := []struct {
		Title    string
		Slug     string
		Content  string
		Category string
		Tags     []string
	}{
		{
			Title:    "Building REST APIs with Go and Gin",
			Slug:     "building-rest-apis-with-go-and-gin",
			Category: "go",
			Tags:     []string{"golang", "gin", "api"},
			Content: `## Introduction

Go is an excellent choice for building REST APIs. Combined with the Gin framework, you can create fast, maintainable, and scalable web services. In this post, I'll walk through building a complete REST API from scratch.

## Why Go for APIs?

Go's standard library already includes a production-ready HTTP server. Its concurrency model makes it easy to handle thousands of simultaneous connections. And the language's simplicity means your codebase stays readable as it grows.

## Setting Up the Project

First, initialize a new Go module:

` + "```bash\n" + `mkdir myapi && cd myapi
go mod init myapi
go get github.com/gin-gonic/gin
` + "```\n\n" + `## Your First Handler

Gin makes routing intuitive. Here's a simple "Hello World" endpoint:

` + "```go\n" + `package main

import (
    "net/http"
    "github.com/gin-gonic/gin"
)

func main() {
    r := gin.Default()

    r.GET("/api/hello", func(c *gin.Context) {
        c.JSON(http.StatusOK, gin.H{
            "message": "Hello, World!",
        })
    })

    r.Run(":8080")
}
` + "```\n\n" + `## Structuring Your API

As your API grows, you'll want to organize it into handlers, models, and middleware. A common structure looks like:

- ` + "`handlers/`" + ` — HTTP request handlers
- ` + "`models/`" + ` — data structures and DB logic
- ` + "`middleware/`" + ` — authentication, logging, CORS

## Error Handling

Always return meaningful error messages:

` + "```go\n" + `func GetUser(c *gin.Context) {
    id := c.Param("id")
    user, err := db.FindUser(id)
    if err != nil {
        c.JSON(http.StatusNotFound, gin.H{
            "error": "User not found",
        })
        return
    }
    c.JSON(http.StatusOK, user)
}
` + "```\n\n" + `## Conclusion

Go and Gin give you everything you need to build robust REST APIs. The performance is excellent, the code is clean, and the ecosystem is mature. Give it a try on your next project!`,
		},
		{
			Title:    "Understanding Goroutines and Channels",
			Slug:     "understanding-goroutines-and-channels",
			Category: "go",
			Tags:     []string{"golang", "concurrency", "goroutines"},
			Content: `## What Are Goroutines?

A goroutine is a lightweight thread managed by the Go runtime. They're cheap to create — you can spawn thousands of them without breaking a sweat. Just prefix any function call with ` + "`go`" + `:

` + "```go\n" + `func sayHello() {
    fmt.Println("Hello from goroutine!")
}

func main() {
    go sayHello()
    time.Sleep(time.Second) // give it time to finish
}
` + "```\n\n" + `## The Power of Channels

Channels are the pipes that connect goroutines. They let you safely pass data between concurrent operations without explicit locks.

` + "```go\n" + `func main() {
    ch := make(chan string)

    go func() {
        ch <- "Hello from goroutine!"
    }()

    msg := <-ch
    fmt.Println(msg)
}
` + "```\n\n" + `## Buffered vs Unbuffered Channels

Unbuffered channels block until both sender and receiver are ready — they're a synchronization primitive. Buffered channels let you queue up values:

` + "```go\n" + `// Unbuffered: synchronous
ch := make(chan int)

// Buffered: capacity of 3
ch := make(chan int, 3)
` + "```\n\n" + `## Select Statement

The ` + "`select`" + ` statement lets a goroutine wait on multiple channel operations:

` + "```go\n" + `select {
case msg1 := <-ch1:
    fmt.Println("Received from ch1:", msg1)
case msg2 := <-ch2:
    fmt.Println("Received from ch2:", msg2)
case <-time.After(time.Second):
    fmt.Println("Timeout!")
}
` + "```\n\n" + `## Common Patterns

### Worker Pool

Limit concurrency by using a pool of worker goroutines:

` + "```go\n" + `func worker(id int, jobs <-chan int, results chan<- int) {
    for j := range jobs {
        results <- j * 2
    }
}

func main() {
    jobs := make(chan int, 100)
    results := make(chan int, 100)

    for w := 1; w <= 3; w++ {
        go worker(w, jobs, results)
    }
}
` + "```\n\n" + `## Key Takeaways

- Goroutines are cheap, lightweight threads
- Channels enable safe communication between goroutines
- Use ` + "`select`" + ` to handle multiple channels
- Always make sure channels are properly closed to avoid goroutine leaks`,
		},
		{
			Title:    "Why I Switched from PostgreSQL to SQLite",
			Slug:     "why-i-switched-from-postgresql-to-sqlite",
			Category: "architecture",
			Tags:     []string{"sqlite", "database"},
			Content: `## The Database That Surprised Me

For years, I used PostgreSQL for every project by default. It's powerful, reliable, and battle-tested. But recently, I made a surprising switch — and I'm not going back.

## The Realization

Most applications don't need a client-server database. Here's what I discovered:

- **My blog gets ~1000 visitors/day.** SQLite handles 100x that easily.
- **I'm the only writer.** There's no concurrent write contention.
- **Deployment simplicity matters.** One binary, one file, zero configuration.

## SQLite's Hidden Strengths

### Performance

For read-heavy workloads, SQLite often outperforms PostgreSQL because there's no network overhead:

` + "```bash\n" + `# SQLite: direct function calls
# PostgreSQL: TCP round-trip per query
` + "```\n\n" + `### Zero Administration

No ` + "`pg_hba.conf`" + `, no user management, no ` + "`VACUUM`" + ` cron jobs. The database is just a file:

` + "```bash\n" + `ls -lh blog.db
# -rw-r--r-- 1 kyle staff 2.4M blog.db
` + "```\n\n" + `Back up the entire database with ` + "`cp blog.db backup.db`" + `. Try doing that with a 50GB PostgreSQL cluster.

### Reliability

SQLite is the most tested software component in the world. The test suite has over 100 million lines of test code. It's used in every iPhone, Android device, and web browser.

## When NOT to Use SQLite

SQLite isn't right for everything:

1. **High write concurrency** — SQLite serializes writes
2. **Multiple application servers** — you need network access to the DB file
3. **Very large datasets** — while SQLite supports terabytes, administrative tools are sparse

## My Setup

` + "```go\n" + `import "github.com/mattn/go-sqlite3"

db, err := sql.Open("sqlite3", "blog.db?_journal_mode=WAL&_foreign_keys=on")
` + "```\n\n" + `WAL mode gives me concurrent reads while a write is happening — perfect for a blog where writes are rare and reads are constant.

## Conclusion

SQLite is not a toy. For single-server applications, it's often the right choice. Don't reach for PostgreSQL just because it's what you've always used. Think about what your application actually needs.`,
		},
		{
			Title:    "A Practical Guide to Docker Multi-Stage Builds",
			Slug:     "a-practical-guide-to-docker-multi-stage-builds",
			Category: "devops",
			Tags:     []string{"docker", "devops"},
			Content: `## The Problem with Docker Images

A typical Go application compiles to a 10MB binary. But a naive Docker image can easily balloon to 800MB+ because it includes the entire Go toolchain and build dependencies.

## Enter Multi-Stage Builds

Multi-stage builds let you use one image to build your application and a different, minimal image to run it:

` + "```dockerfile\n" + `# Stage 1: Build
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o server .

# Stage 2: Run
FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /app/server /server
EXPOSE 8080
CMD ["/server"]
` + "```\n\n" + `The final image is only ~15MB — the size of Alpine plus your binary.

## Why This Matters

Smaller images mean:

- **Faster deployments** — pulling 15MB vs 800MB is a huge difference
- **Better security** — fewer packages = fewer CVEs
- **Lower costs** — less storage and bandwidth in your container registry

## A More Advanced Example

For a React frontend with a Go backend:

` + "```dockerfile\n" + `# Stage 1: Build frontend
FROM node:20-alpine AS frontend
WORKDIR /app
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ .
RUN npm run build

# Stage 2: Build backend
FROM golang:1.23-alpine AS backend
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/dist ./static
RUN CGO_ENABLED=0 go build -o server .

# Stage 3: Run
FROM scratch
COPY --from=backend /app/server /server
COPY --from=backend /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
EXPOSE 8080
CMD ["/server"]
` + "```\n\n" + `Using ` + "`FROM scratch`" + ` gives you the absolute minimal image — just your binary and nothing else.

## Pro Tips

1. **Order your COPY commands carefully** — Docker caches layers. Copy ` + "`go.mod`" + ` and ` + "`go.sum`" + ` first, then run ` + "`go mod download`" + `, THEN copy source code. This way, dependency downloads are cached unless dependencies change.

2. **Use ` + "`.dockerignore`" + `** — exclude ` + "`node_modules`" + `, ` + "`.git`" + `, and build artifacts from your build context:

` + "```\n" + `node_modules
.git
*.log
dist
` + "```\n\n" + `3. **Don't run as root** — add a non-root user in your final stage:

` + "```dockerfile\n" + `RUN addgroup -S app && adduser -S app -G app
USER app
` + "```\n\n" + `## Wrapping Up

Multi-stage builds are one of Docker's best features. They keep your images small, secure, and production-ready without complex build scripts. If you're not using them yet, start today!`,
		},
		{
			Title:    "TypeScript Generics Explained Simply",
			Slug:     "typescript-generics-explained-simply",
			Category: "javascript",
			Tags:     []string{"typescript", "javascript", "react"},
			Content: `## Why Generics?

Without generics, you write code like this:

` + "```typescript\n" + `function getFirst(arr: any[]): any {
    return arr[0];
}

const num = getFirst([1, 2, 3]); // type is 'any' — we lost information!
` + "```\n\n" + `We know it returns a number, but TypeScript doesn't. This is where generics come in.

## Your First Generic

` + "```typescript\n" + `function getFirst<T>(arr: T[]): T | undefined {
    return arr[0];
}

const num = getFirst([1, 2, 3]);    // type is 'number'
const str = getFirst(["a", "b"]);    // type is 'string'
` + "```\n\n" + `The ` + "`<T>`" + ` is a type parameter — it captures whatever type the caller provides, and uses it in the return type.

## Generic Constraints

Sometimes you want to restrict what types are allowed:

` + "```typescript\n" + `interface HasLength {
    length: number;
}

function logLength<T extends HasLength>(item: T): T {
    console.log(item.length);
    return item;
}

logLength("hello");     // OK, string has .length
logLength([1, 2, 3]);   // OK, array has .length
logLength(42);          // Error! number doesn't have .length
` + "```\n\n" + `## Generic Interfaces

This is where generics really shine:

` + "```typescript\n" + `interface ApiResponse<T> {
    data: T;
    status: number;
    message: string;
}

interface User {
    id: number;
    name: string;
}

// fetchUser returns ApiResponse<User>
async function fetchUser(id: number): Promise<ApiResponse<User>> {
    const res = await fetch(` + "`/api/users/${id}`" + `);
    return res.json();
}
` + "```\n\n" + `## Real-World Example: A useFetch Hook

` + "```typescript\n" + `function useFetch<T>(url: string) {
    const [data, setData] = useState<T | null>(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<Error | null>(null);

    useEffect(() => {
        fetch(url)
            .then(res => res.json())
            .then((json: T) => {
                setData(json);
                setLoading(false);
            })
            .catch(err => {
                setError(err);
                setLoading(false);
            });
    }, [url]);

    return { data, loading, error };
}

// Usage — TypeScript infers User[] automatically
const { data, loading } = useFetch<User[]>("/api/users");
` + "```\n\n" + `## Key Takeaways

- **` + "`<T>`" + `** is a placeholder for a type that the caller provides
- Use **constraints** (` + "`extends`" + `) to limit what types are accepted
- Generics compose beautifully — ` + "`Promise<ApiResponse<User[]>>`" + ` is perfectly valid
- **Don't overuse generics** — if your function only ever works with strings, just use ` + "`string`" + `

Generics are one of TypeScript's most powerful features. They let you write flexible, reusable code while keeping full type safety. Start using them today!`,
		},
	}

	// Insert posts
	for _, p := range posts {
		// Render markdown to HTML
		unsafe := blackfriday.Run([]byte(p.Content))
		html := string(policy.SanitizeBytes(unsafe))

		// Generate excerpt (first 200 chars of plain text)
		excerpt := extractExcerpt(p.Content, 200)

		// Get category ID
		var catID sql.NullInt64
		err := db.QueryRow("SELECT id FROM categories WHERE slug = ?", p.Category).Scan(&catID)
		if err != nil {
			catID = sql.NullInt64{}
		}

		result, err := db.Exec(
			`INSERT INTO posts (title, slug, excerpt, content_md, content_html, category_id, is_published)
			 VALUES (?, ?, ?, ?, ?, ?, 1)`,
			p.Title, p.Slug, excerpt, p.Content, html, catID,
		)
		if err != nil {
			return err
		}

		postID, _ := result.LastInsertId()

		// Insert post_tags
		for _, tagSlug := range p.Tags {
			var tagID int64
			if err := db.QueryRow("SELECT id FROM tags WHERE slug = ?", tagSlug).Scan(&tagID); err != nil {
				continue
			}
			db.Exec("INSERT OR IGNORE INTO post_tags (post_id, tag_id) VALUES (?, ?)", postID, tagID)
		}
	}

	log.Printf("Seeded %d posts successfully", len(posts))
	return nil
}

func extractExcerpt(md string, maxLen int) string {
	// Remove markdown syntax roughly
	md = strings.ReplaceAll(md, "`", "")
	md = strings.ReplaceAll(md, "#", "")
	md = strings.ReplaceAll(md, "*", "")
	md = strings.ReplaceAll(md, "_", "")
	md = strings.ReplaceAll(md, "[", "")
	md = strings.ReplaceAll(md, "]", "")
	md = strings.ReplaceAll(md, "(", "")
	md = strings.ReplaceAll(md, ")", "")
	md = strings.ReplaceAll(md, "```", "")

	// Normalize whitespace
	md = strings.Join(strings.Fields(md), " ")

	if len(md) > maxLen {
		// Try to break at a word boundary
		cut := md[:maxLen]
		if lastSpace := strings.LastIndex(cut, " "); lastSpace > 0 {
			cut = cut[:lastSpace]
		}
		return cut + "..."
	}
	return md
}
