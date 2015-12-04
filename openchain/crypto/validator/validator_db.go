/*
Licensed to the Apache Software Foundation (ASF) under one
or more contributor license agreements.  See the NOTICE file
distributed with this work for additional information
regarding copyright ownership.  The ASF licenses this file
to you under the Apache License, Version 2.0 (the
"License"); you may not use this file except in compliance
with the License.  You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing,
software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
KIND, either express or implied.  See the License for the
specific language governing permissions and limitations
under the License.
*/

package validator

import (
	"database/sql"
	"errors"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"github.com/openblockchain/obc-peer/openchain/crypto/utils"
	"os"
	"path/filepath"
)

var ErrDBAlreadyInitialized error = errors.New("DB already Initilized.")

type DB struct {
	sqlDB *sql.DB
}

func (db *DB) Init() error {
	return nil
}

func (db *DB) GetEnrollmentCert(id []byte, certFetcher func(id []byte) ([]byte, error)) ([]byte, error) {
	sid := utils.EncodeBase64(id)

	cert, err := db.selectEnrollmentCert(sid)
	if err != nil {
		log.Error("Failed selecting enrollment cert: %s", err)

		return nil, err
	}
	log.Info("GetEnrollmentCert:cert %s", utils.EncodeBase64(cert))

	if cert == nil {
		// If No cert is available, fetch from ECA

		// 1. Fetch
		log.Info("Fectch Enrollment Certificate from ECA...")
		cert, err = certFetcher(id)
		if err != nil {
			return nil, err
		}

		// 2. Store
		log.Info("Store certificate...")
		tx, err := db.sqlDB.Begin()
		if err != nil {
			log.Error("Failed beginning transaction: %s", err)

			return nil, err
		}

		log.Info("Insert id %s", sid)
		log.Info("Insert cert %s", utils.EncodeBase64(cert))

		_, err = tx.Exec("INSERT INTO Certificates (id, cert) VALUES (?, ?)", sid, cert)

		if err != nil {
			log.Error("Failed inserting cert %s", err)

			tx.Rollback()

			return nil, err
		}

		err = tx.Commit()
		if err != nil {
			log.Error("Failed committing transaction: %s", err)

			tx.Rollback()

			return nil, err
		}

		log.Info("Fectch Enrollment Certificate from ECA...done!")

		cert, err = db.selectEnrollmentCert(sid)
		if err != nil {
			log.Error("Failed selecting next TCert after fetching: %s", err)

			return nil, err
		}
	}

	return cert, nil
}

func (db *DB) CloseDB() {
	db.sqlDB.Close()
	isOpen = false
}

func (db *DB) selectEnrollmentCert(id string) ([]byte, error) {
	log.Info("Select Enrollment TCert...")

	// Get the first row available
	var cert []byte
	row := db.sqlDB.QueryRow("SELECT cert FROM Certificates where id = ?", id)
	err := row.Scan(&cert)

	if err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		log.Error("Error during select: %s", err)

		return nil, err
	}

	log.Info("cert %s", utils.EncodeBase64(cert))

	log.Info("Select Enrollment Cert...done!")

	return cert, nil
}

var db *DB
var isOpen bool

func initDB() error {
	// TODO: applay syncronization
	if isOpen {
		return errors.New("DB already initialized.")
	}

	err := createDBIfDBPathEmpty()
	if err != nil {
		return err
	}

	db, err = openDB()
	if err != nil {
		return err
	}
	return nil
}

// CreateDB creates a ca db database
func createDB() error {
	dbPath := getDBPath()
	log.Debug("Creating DB at [%s]", dbPath)

	missing, err := utils.FileMissing(dbPath, getDBFilename())
	if !missing {
		return fmt.Errorf("db dir [%s] already exists", dbPath)
	}

	os.MkdirAll(dbPath, 0755)

	log.Debug("Open DB at [%s]", dbPath)
	db, err := sql.Open("sqlite3", filepath.Join(dbPath, getDBName()))
	if err != nil {
		return err
	}

	log.Debug("Ping DB at [%s]", dbPath)
	err = db.Ping()
	if err != nil {
		log.Fatal(err)
	}

	defer db.Close()

	// create tables
	log.Debug("Create Table [%s] at [%s]", "Certificates", dbPath)
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS Certificates (id VARCHAR, cert BLOB, PRIMARY KEY (id))"); err != nil {
		log.Debug("Failed creating table: %s", err)
		return err
	}

	log.Debug("DB created at [%s]", dbPath)
	return nil
}

// DeleteDB deletes a ca db database
func deleteDB() error {
	log.Debug("Removing DB at [%s]", getDBPath())

	return os.RemoveAll(getDBPath())
}

// GetDBHandle returns a handle to db
func getDBHandle() *DB {
	return db
}

func getDBName() string {
	return "client.db"
}

func createDBIfDBPathEmpty() error {
	// Check directory
	dbPath := getDBPath()
	missing, err := utils.DirMissingOrEmpty(dbPath)
	if err != nil {
		log.Error("Failed checking directory %s: %s", dbPath, err)
	}
	log.Debug("Db path [%s] missing [%t]", dbPath, missing)

	if !missing {
		// Check file
		missing, err = utils.FileMissing(getDBPath(), getDBName())
		if err != nil {
			log.Error("Failed checking file %s: %s", getDBFilePath(), err)
		}
		log.Debug("Db file [%s] missing [%t]", getDBFilePath(), missing)
	}

	if missing {
		err := createDB()
		if err != nil {
			log.Debug("Failed creating db At [%s]: %s", getDBFilePath(), err.Error())
			return nil
		}
	}

	return nil
}

func openDB() (*DB, error) {
	if isOpen {
		return db, nil
	}
	dbPath := getDBPath()

	sqlDB, err := sql.Open("sqlite3", filepath.Join(dbPath, getDBName()))

	if err != nil {
		log.Error("Error opening DB", err)
		return nil, err
	}
	isOpen = true

	return &DB{sqlDB}, nil
}
