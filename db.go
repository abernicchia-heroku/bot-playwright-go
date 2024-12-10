// reference(s):
// 	https://github.com/heroku/sql-drain
// 	debug test bug - https://github.com/golang/vscode-go/issues/2953

package main

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

const DbUrlEnv string = "DATABASE_URL"

var db *sql.DB
var carModelYEntryInsertStmt *sql.Stmt
var carModel3EntryInsertStmt *sql.Stmt

const createTableTemplate string = `
	CREATE TABLE IF NOT EXISTS ${TABLENAME} (
		id SERIAL,
		time timestamp NOT NULL,
		price float NOT NULL,
		PRIMARY KEY (id)
  );
  	CREATE INDEX IF NOT EXISTS ${TABLENAME}_time_idx ON ${TABLENAME}(time);
`

const insertCarModelPriceTemplate string = "INSERT into ${TABLENAME}(time, price) VALUES ($1, $2);"

func carModelPriceInsert(carModelType CarModelType, time time.Time, price float64) error {
	if isEnvGreaterThan(DebugEnv, 1000) {
		fmt.Printf("[db.go:carModelPriceInsert] inserting price entry type[%s] date[%v] cost[%v]\n", carModelType.String(), time, price)
	}

	var entryInsertStmt *sql.Stmt

	if carModelType == CarModelType_MY {
		entryInsertStmt = carModelYEntryInsertStmt
	} else if carModelType == CarModelType_M3 {
		entryInsertStmt = carModel3EntryInsertStmt
	}

	_, err := entryInsertStmt.Exec(
		time,
		price)
	if err != nil {
		fmt.Printf("[db.go:costEntryInsert] DB error: %v\n", err)
	}

	return err
}

// called when the class is loaded
func init() {
	// Connect to postgresql
	var err error

	dburl := os.Getenv(DbUrlEnv) + "?sslmode=require&application_name=bot-playwright-go"

	u, err := url.Parse(dburl)
	if err != nil {
		fmt.Printf("Invalid DB URL: %v\n", err)
	}

	if isEnv(DebugEnv) {
		fmt.Printf("[db.go:init] db url %v\n", u.Redacted())
	}

	db, err = sql.Open("postgres", dburl)
	if err != nil {
		fmt.Printf("Open DB error: %v\n", err)
	}

	err = db.Ping()
	if err != nil {
		fmt.Printf("Unable to ping DB: %v\n", err)
	}

	fmt.Printf("Initializing db tables ...\n")

	_, err = db.Exec(strings.Replace(createTableTemplate, "${TABLENAME}", CarModelType_MY.String(), -1))
	if err != nil {
		fmt.Printf("Unable to create [model_y] table: %v\n", err)
	}

	_, err = db.Exec(strings.Replace(createTableTemplate, "${TABLENAME}", CarModelType_M3.String(), -1))
	if err != nil {
		fmt.Printf("Unable to create [model_3] table: %v\n", err)
	}

	// tables need to be created before the prepared statements are created as they depend on them
	fmt.Printf("Initializing prepared statements ...\n")

	carModelYEntryInsertStmt, err = db.Prepare(strings.Replace(insertCarModelPriceTemplate, "${TABLENAME}", CarModelType_MY.String(), -1))
	if err != nil {
		fmt.Printf("Unable to create prepared stmt: %v\n", err)
	}

	carModel3EntryInsertStmt, err = db.Prepare(strings.Replace(insertCarModelPriceTemplate, "${TABLENAME}", CarModelType_M3.String(), -1))
	if err != nil {
		fmt.Printf("Unable to create prepared stmt: %v\n", err)
	}
}
