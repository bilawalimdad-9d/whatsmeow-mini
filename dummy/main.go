package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	_ "github.com/mattn/go-sqlite3"
	"go.mau.fi/whatsmeow"
	waBinary "go.mau.fi/whatsmeow/binary"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
)

func eventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		fmt.Printf("Received a message! %s\n", v.Info.ID)
	case *events.Receipt:
		fmt.Printf("Received a receipt! %v\n", v.MessageIDs)
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: dummy <session.json>")
		os.Exit(1)
	}
	dbLog := waLog.Stdout("Database", "DEBUG", true)
	// Just use a dummy in-memory or throwaway sqlite to store the hydrated device
	container, err := sqlstore.New(context.Background(), "sqlite3", "file:dummy.db?_foreign_keys=on", dbLog)
	if err != nil {
		panic(err)
	}

	sessionFile := os.Args[1]
	data, err := os.ReadFile(sessionFile)
	if err != nil {
		panic(fmt.Errorf("failed to read session JSON: %w", err))
	}

	// 1. Unmarshal JSON into a new device
	deviceStore := container.NewDevice()
	if err := json.Unmarshal(data, deviceStore); err != nil {
		panic(fmt.Errorf("failed to parse JSON: %w", err))
	}

	// Important: save the restored device to the database
	if err := deviceStore.Save(context.Background()); err != nil {
		panic(fmt.Errorf("failed to save hydrated device to db: %w", err))
	}

	clientLog := waLog.Stdout("Client", "DEBUG", true)
	client := whatsmeow.NewClient(deviceStore, clientLog)

	// 2. Observer: no delivery <receipt>; message stanza ack skipped via RawMessageHook early return.
	// Transport <ack> for notification/receipt/etc. stays enabled (DisableAcks=false).
	// DisableKeyManagement=true prevents pre-key pool poisoning (observer doesn't share key store with real device).
	client.DisableAcks = false
	client.DisableReceipts = true
	client.DisableKeyManagement = true

	// 3. Set the raw webhook! (This short-circuits all parsing/decryption)
	client.RawMessageHook = func(messageID string, sender types.JID, rawNode *waBinary.Node) {
		fmt.Printf("Raw webhook fired! MsgID: %s | Sender: %s\n", messageID, sender)
	}

	client.AddEventHandler(eventHandler)

	if err := client.Connect(); err != nil {
		panic(err)
	}

	// Listen to Ctrl+C (you can also do something else that prevents the program from exiting)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	client.Disconnect()
}
