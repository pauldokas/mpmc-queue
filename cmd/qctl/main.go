package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"mpmc-queue/queue"
)

var (
	queues    = make(map[string]*queue.Queue)
	consumers = make(map[string]*queue.Consumer)
)

func main() {
	fmt.Println("mpmc-queue CLI (qctl)")
	fmt.Println("Type 'help' for commands")

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}
		line := scanner.Text()
		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}

		cmd := parts[0]
		args := parts[1:]

		switch cmd {
		case "create":
			handleCreate(args)
		case "list":
			handleList()
		case "stats":
			handleStats(args)
		case "pub":
			handlePub(args)
		case "sub":
			handleSub(args)
		case "help":
			handleHelp()
		case "exit", "quit":
			fmt.Println("Bye!")
			return
		default:
			fmt.Println("Unknown command. Type 'help'.")
		}
	}
}

func handleHelp() {
	fmt.Println(`
Commands:
  create <name> [ttl_sec]  Create a new queue (default 1MB limit)
  list                     List all active queues
  stats <name>             Show stats for a queue
  pub <name> <data...>     Enqueue a string message
  sub <name>               Create a background consumer that prints messages
  exit                     Exit the shell
`)
}

func handleCreate(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: create <name> [ttl_sec]")
		return
	}
	name := args[0]
	if _, exists := queues[name]; exists {
		fmt.Printf("Queue '%s' already exists\n", name)
		return
	}

	ttl := 10 * time.Minute
	if len(args) > 1 {
		sec, err := strconv.Atoi(args[1])
		if err == nil {
			ttl = time.Duration(sec) * time.Second
		}
	}

	q := queue.NewQueueWithTTL(name, ttl)
	queues[name] = q
	fmt.Printf("Created queue '%s' with TTL %v\n", name, ttl)
}

func handleList() {
	if len(queues) == 0 {
		fmt.Println("No queues.")
		return
	}
	for name, q := range queues {
		stats := q.GetQueueStats()
		fmt.Printf("- %s: %d items, %d consumers\n", name, stats.TotalItems, stats.ConsumerCount)
	}
}

func handleStats(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: stats <name>")
		return
	}
	name := args[0]
	q, ok := queues[name]
	if !ok {
		fmt.Printf("Queue '%s' not found\n", name)
		return
	}

	stats := q.GetQueueStats()
	fmt.Printf("=== Stats: %s ===\n", stats.Name)
	fmt.Printf("Items:       %d\n", stats.TotalItems)
	fmt.Printf("Memory:      %d bytes (%.2f%%)\n", stats.MemoryUsage, stats.MemoryPercent)
	fmt.Printf("Consumers:   %d\n", stats.ConsumerCount)
	fmt.Printf("TTL:         %v\n", stats.TTL)
	fmt.Printf("Age:         %v\n", time.Since(stats.CreatedAt))
}

func handlePub(args []string) {
	if len(args) < 2 {
		fmt.Println("Usage: pub <name> <message...>")
		return
	}
	name := args[0]
	msg := strings.Join(args[1:], " ")

	q, ok := queues[name]
	if !ok {
		fmt.Printf("Queue '%s' not found\n", name)
		return
	}

	if err := q.TryEnqueue(msg); err != nil {
		fmt.Printf("Enqueue failed: %v\n", err)
	} else {
		fmt.Println("Message enqueued.")
	}
}

func handleSub(args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: sub <name>")
		return
	}
	name := args[0]
	q, ok := queues[name]
	if !ok {
		fmt.Printf("Queue '%s' not found\n", name)
		return
	}

	consumer := q.AddConsumer()
	id := consumer.GetID()
	consumers[id] = consumer

	fmt.Printf("Started consumer %s. Messages will appear below...\n", id[:8])

	go func() {
		for {
			data := consumer.Read()
			if data == nil {
				fmt.Printf("[Consumer %s] Queue closed or stopped.\n", id[:8])
				return
			}
			fmt.Printf("[Consumer %s read from %s]: %v\n> ", id[:8], name, data.Payload)
		}
	}()
}
