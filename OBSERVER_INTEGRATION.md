# Whatsmeow Observer Mode (Stripped-Down Integration)

This is a customized, **read-only / observer fork** of the `whatsmeow` WhatsApp Web API library. 

This modified library is specifically designed to allow you to connect a linked companion device session without affecting the message queue on the WhatsApp servers. It prevents the library from automatically acknowledging messages or sending delivery receipts. This means your primary `whatsmeow` instance will still receive all messages normally when it connects!

## Key Features Added

1. **`DisableAcks` Flag**: When set to `true`, the client drops all automated `<ack>` stanzas for incoming binary nodes.
2. **`DisableReceipts` Flag**: When set to `true`, the client drops all standard delivery receipts and retry receipts.
3. **`RawMessageHook` Function**: A short-circuit callback that bypasses all Signal protocol decryption, SQLite queries, and unmarshalling pipelines. It intercepts the raw binary frame, passes you the Message ID and Sender, and immediately returns—saving massive CPU overhead and avoiding unintended state mutations.

## How to Import and Use in Your Main Project

If you have cloned or added this repository as a submodule to your main project, you can map it using the `replace` directive in your Go backend.

### 1. `go.mod` Setup

In your main project's directory, open your `go.mod` file and add the `replace` directive to point to this modified clone:

```go
module your-backend-service

go 1.21

require go.mau.fi/whatsmeow v0.0.0-latest

// Point this path to wherever you cloned this customized repo!
replace go.mau.fi/whatsmeow => ./path/to/cloned/Whatsmeow-mini/whatsmeow
```

Run `go mod tidy` to ensure Go picks up the local replacement.

### 2. Implementation Code

To successfully open a connection on an existing session and trigger your webhooks on incoming messages (new messages, edits, reactions):

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
	
	// Ensure you import the sqlite driver for the device store hydration
	_ "github.com/mattn/go-sqlite3" 
)

func main() {
	// 1. Initialize an in-memory or temporary database for the hydrated session
	dbLog := waLog.Stdout("Database", "DEBUG", true)
	container, err := sqlstore.New(context.Background(), "sqlite3", "file:observer.db?_foreign_keys=on", dbLog)
	if err != nil {
		panic(err)
	}

	// 2. Hydrate the exported JSON session from your primary client
	data, err := os.ReadFile("session.json")
	if err != nil {
		panic(err)
	}
	
	deviceStore := container.NewDevice()
	if err := json.Unmarshal(data, deviceStore); err != nil {
		panic(err)
	}
	deviceStore.Save(context.Background())

	// 3. Initialize the Observer Client
	clientLog := waLog.Stdout("Client", "DEBUG", true)
	client := whatsmeow.NewClient(deviceStore, clientLog)

	// ==========================================
	// 4. ACTIVATE OBSERVER / STRIPPED DOWN MODE
	// ==========================================
	
	// Prevent Acks and Delivery Receipts
	client.DisableAcks = true
	client.DisableReceipts = true

	// Hook the raw binary frame directly, bypassing decryption!
	client.RawMessageHook = func(messageID string, sender types.JID, rawNode *waBinary.Node) {
		// This webhook fires for new messages, edits, and reactions!
		// It safely ignores calls, presence updates, and chat states.
		
		fmt.Printf("TRIGGER: New Message Event | ID: %s | Sender: %s\n", messageID, sender.String())
		
		// --> FIRE YOUR BACKEND SERVICES OR WEBHOOK URL HERE <--
	}

	// 5. Connect to WhatsApp Web Socket!
	if err := client.Connect(); err != nil {
		panic(err)
	}

	// Keep the service running
	select {}
}
```

### Notes & Limitations

* Because decryption requires sequential tracking of the Signal protocol (`old counter` issues) and involves database updates, utilizing `RawMessageHook` completely strips away decryption. The `rawNode` passed to your hook will be encrypted. 
* This is exactly what is needed for an **event trigger** service—you know *who* sent *something*, but you leave the actual content parsing to your primary `whatsmeow` instance when it reconnects.
* Push Notifications via FCM (`RegisterForPushNotifications`) can be used, but since WhatsApp Web instances don't route high-priority mobile ringing pushes, WebSockets are vastly superior for capturing the rapid `<message>` events your webhook relies on.
