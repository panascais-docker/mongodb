package main

import (
	"context"
	"errors"
	"net"
	"slices"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	securedEarlier     = 13
	alreadyInitialized = 23
	userExists         = 51003
)

func dial(port int, login *credential) (*mongo.Client, error) {
	clientOptions := options.Client().
		SetHosts([]string{net.JoinHostPort(loopback, strconv.Itoa(port))}).
		SetDirect(true).
		SetServerSelectionTimeout(time.Second)

	if login != nil {
		clientOptions.SetAuth(options.Credential{Username: login.username, Password: login.password, AuthSource: "admin"})
	}

	return mongo.Connect(clientOptions)
}

func connected(port int, use func(context.Context, *mongo.Client) error) step {
	return func(ctx context.Context) error {
		client, err := dial(port, nil)
		if err != nil {
			return err
		}
		defer func() { _ = client.Disconnect(context.Background()) }()

		if err := waitUntil(ctx, func() error { return client.Ping(ctx, nil) }); err != nil {
			return err
		}

		return use(ctx, client)
	}
}

func runAdmin(ctx context.Context, client *mongo.Client, command bson.D) *mongo.SingleResult {
	return client.Database("admin").RunCommand(ctx, command)
}

func waitPrimary(ctx context.Context, client *mongo.Client) error {
	return waitUntil(ctx, func() error {
		var hello struct {
			IsWritablePrimary bool `bson:"isWritablePrimary"`
		}

		if err := runAdmin(ctx, client, bson.D{{Key: "hello", Value: 1}}).Decode(&hello); err != nil {
			return err
		}

		if !hello.IsWritablePrimary {
			return errors.New("no writable primary yet")
		}

		return nil
	})
}

func waitUntil(ctx context.Context, check func() error) error {
	for {
		err := check()
		if err == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return err
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func ignoreCodes(err error, codes ...int32) error {
	if commandError, failed := errors.AsType[mongo.CommandError](err); failed && slices.Contains(codes, commandError.Code) {
		return nil
	}

	return err
}
