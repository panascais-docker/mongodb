package main

import (
	"context"
	"errors"
	"log"
	"net"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	unauthorizedCode       = 13
	alreadyInitializedCode = 23
	userAlreadyExistsCode  = 51003
)

type credential struct {
	username string
	password string
}

func loadRootCredential() *credential {
	username, password := secret("MONGODB_ROOT_USERNAME"), secret("MONGODB_ROOT_PASSWORD")
	if username == "" && password == "" {
		return nil
	}

	if username == "" || password == "" {
		log.Fatal("MONGODB_ROOT_USERNAME and MONGODB_ROOT_PASSWORD must be set together")
	}

	return &credential{username: username, password: password}
}

func secret(name string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}

	path := os.Getenv(name + "_FILE")
	if path == "" {
		return ""
	}

	value, err := os.ReadFile(path)
	if err != nil {
		log.Fatal(err)
	}

	return strings.TrimRight(string(value), "\r\n")
}

func dial(port int, login *credential) (*mongo.Client, error) {
	clientOptions := options.Client().
		SetHosts([]string{net.JoinHostPort("127.0.0.1", strconv.Itoa(port))}).
		SetDirect(true).
		SetServerSelectionTimeout(time.Second)

	if login != nil {
		clientOptions.SetAuth(options.Credential{Username: login.username, Password: login.password, AuthSource: "admin"})
	}

	return mongo.Connect(clientOptions)
}

func waitReady(ctx context.Context, client *mongo.Client) error {
	return waitUntil(ctx, func() error { return client.Ping(ctx, nil) })
}

func waitPrimary(ctx context.Context, client *mongo.Client) error {
	return waitUntil(ctx, func() error {
		var hello struct {
			IsWritablePrimary bool `bson:"isWritablePrimary"`
		}

		if err := client.Database("admin").RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&hello); err != nil {
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
		if err == nil || ctx.Err() != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return err
		case <-time.After(250 * time.Millisecond):
		}
	}
}

func initiateReplicaSet(ctx context.Context, client *mongo.Client, name, host string, configServer bool) error {
	members := bson.A{bson.D{{Key: "_id", Value: 0}, {Key: "host", Value: host}}}

	configuration := bson.D{{Key: "_id", Value: name}, {Key: "members", Value: members}}
	if configServer {
		configuration = append(configuration, bson.E{Key: "configsvr", Value: true})
	}

	err := client.Database("admin").RunCommand(ctx, bson.D{{Key: "replSetInitiate", Value: configuration}}).Err()

	return ignoreCodes(err, alreadyInitializedCode, unauthorizedCode)
}

func addShard(ctx context.Context, client *mongo.Client, shard string) error {
	err := client.Database("admin").RunCommand(ctx, bson.D{{Key: "addShard", Value: shard}}).Err()

	return ignoreCodes(err, unauthorizedCode)
}

func ignoreCodes(err error, codes ...int32) error {
	if commandError, failed := errors.AsType[mongo.CommandError](err); failed && slices.Contains(codes, commandError.Code) {
		return nil
	}

	return err
}

func createRoot(ctx context.Context, client *mongo.Client, root credential) (bool, error) {
	err := client.Database("admin").RunCommand(ctx, bson.D{
		{Key: "createUser", Value: root.username},
		{Key: "pwd", Value: root.password},
		{Key: "roles", Value: bson.A{bson.D{{Key: "role", Value: "root"}, {Key: "db", Value: "admin"}}}},
	}).Err()
	if err != nil {
		return false, ignoreCodes(err, unauthorizedCode, userAlreadyExistsCode)
	}

	return true, nil
}
