package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type step func(context.Context) error

func initiate(port int, name, host string, configServer bool) step {
	configuration := bson.D{
		{Key: "_id", Value: name},
		{Key: "members", Value: bson.A{bson.D{{Key: "_id", Value: 0}, {Key: "host", Value: host}}}},
	}
	if configServer {
		configuration = append(configuration, bson.E{Key: "configsvr", Value: true})
	}

	return connected(port, func(ctx context.Context, client *mongo.Client) error {
		err := runAdmin(ctx, client, bson.D{{Key: "replSetInitiate", Value: configuration}}).Err()
		if err := ignoreCodes(err, alreadyInitialized, securedEarlier); err != nil {
			return fmt.Errorf("initiating %s: %w", name, err)
		}

		return waitPrimary(ctx, client)
	})
}

func addShard(port int, shard string) step {
	return connected(port, func(ctx context.Context, client *mongo.Client) error {
		return ignoreCodes(runAdmin(ctx, client, bson.D{{Key: "addShard", Value: shard}}).Err(), securedEarlier)
	})
}

func createRoot(port int, root *credential) step {
	if root == nil {
		return func(context.Context) error { return nil }
	}

	return connected(port, func(ctx context.Context, client *mongo.Client) error {
		err := runAdmin(ctx, client, bson.D{
			{Key: "createUser", Value: root.username},
			{Key: "pwd", Value: root.password},
			{Key: "roles", Value: bson.A{bson.D{{Key: "role", Value: "root"}, {Key: "db", Value: "admin"}}}},
		}).Err()
		if err == nil {
			log.Printf("created root user %q", root.username)
		}

		return ignoreCodes(err, securedEarlier, userExists)
	})
}

func markReady(port int) step {
	return func(context.Context) error {
		if err := os.WriteFile(readyFile, []byte(strconv.Itoa(port)), 0o644); err != nil {
			return err
		}

		log.Print("ready")

		return nil
	}
}
