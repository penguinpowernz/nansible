// Copyright 2015 Alex Browne.  All rights reserved.
// Use of this source code is governed by the MIT
// license, which can be found in the LICENSE file.

// File collection.go contains code related to the Collection type.
// This includes all of the most basic operations like Save and Find.
// The Register method and associated methods are also included here.

package zoom

import (
	"container/list"
	"fmt"
	"reflect"
	"strings"

	"github.com/garyburd/redigo/redis"
)

var collections = list.New()

// Collection represents a specific registered type of model. It has methods
// for saving, finding, and deleting models of a specific type. Use the
// NewCollection method to create a new collection.
type Collection struct {
	spec  *modelSpec
	pool  *Pool
	index bool
}

// CollectionOptions contains various options for a pool.
type CollectionOptions struct {
	// FallbackMarshalerUnmarshaler is used to marshal/unmarshal any type into a
	// slice of bytes which is suitable for storing in the database. If Zoom does
	// not know how to directly encode a certain type into bytes, it will use the
	// FallbackMarshalerUnmarshaler. Zoom provides GobMarshalerUnmarshaler and
	// JSONMarshalerUnmarshaler out of the box. You are also free to write your
	// own implementation.
	FallbackMarshalerUnmarshaler MarshalerUnmarshaler
	// If Index is true, any model in the collection that is saved will be added
	// to a set in Redis which acts as an index on all models in the collection.
	// The key for the set is exposed via the IndexKey method. Queries and the
	// FindAll, Count, and DeleteAll methods will not work for unindexed
	// collections. This may change in future versions.
	Index bool
	// Name is a unique string identifier to use for the collection in Redis. All
	// models in this collection that are saved in the database will use the
	// collection name as a prefix. If Name is an empty string, Zoom will use the
	// name of the concrete model type, excluding package prefix and pointer
	// declarations, as the name for the collection. So for example, the default
	// name corresponding to *models.User would be "User". If a custom name is
	// provided, it cannot contain a colon.
	Name string
}

// DefaultCollectionOptions is the default set of options for a collection.
var DefaultCollectionOptions = CollectionOptions{
	FallbackMarshalerUnmarshaler: GobMarshalerUnmarshaler,
	Index: false,
	Name:  "",
}

// WithFallbackMarshalerUnmarshaler returns a new copy of the options with the
// FallbackMarshalerUnmarshaler property set to the given value. It does not
// mutate the original options.
func (options CollectionOptions) WithFallbackMarshalerUnmarshaler(fallback MarshalerUnmarshaler) CollectionOptions {
	options.FallbackMarshalerUnmarshaler = fallback
	return options
}

// WithIndex returns a new copy of the options with the Index property set to
// the given value. It does not mutate the original options.
func (options CollectionOptions) WithIndex(index bool) CollectionOptions {
	options.Index = index
	return options
}

// WithName returns a new copy of the options with the Name property set to the
// given value. It does not mutate the original options.
func (options CollectionOptions) WithName(name string) CollectionOptions {
	options.Name = name
	return options
}

// NewCollection registers and returns a new collection of the given model type.
// You must create a collection for each model type you want to save. The type
// of model must be unique, i.e., not already registered, and must be a pointer
// to a struct. NewCollection will use all the default options for the
// collection, which are specified in DefaultCollectionOptions. If you want to
// specify different options, use the NewCollectionWithOptions method.
func (p *Pool) NewCollection(model Model) (*Collection, error) {
	return p.NewCollectionWithOptions(model, DefaultCollectionOptions)
}

// NewCollectionWithOptions registers and returns a new collection of the given
// model type and with the provided options.
func (p *Pool) NewCollectionWithOptions(model Model, options CollectionOptions) (*Collection, error) {
	typ := reflect.TypeOf(model)
	// If options.Name is empty use the name of the concrete model type (without
	// the package prefix).
	if options.Name == "" {
		options.Name = getDefaultModelSpecName(typ)
	} else if strings.Contains(options.Name, ":") {
		return nil, fmt.Errorf("zoom: CollectionOptions.Name cannot contain a colon. Got: %s", options.Name)
	}

	// Make sure the name and type have not been previously registered
	switch {
	case p.typeIsRegistered(typ):
		return nil, fmt.Errorf("zoom: Error in NewCollection: The type %T has already been registered", model)
	case p.nameIsRegistered(options.Name):
		return nil, fmt.Errorf("zoom: Error in NewCollection: The name %s has already been registered", options.Name)
	case !typeIsPointerToStruct(typ):
		return nil, fmt.Errorf("zoom: NewCollection requires a pointer to a struct as an argument. Got type %T", model)
	}

	// Compile the spec for this model and store it in the maps
	spec, err := compileModelSpec(typ)
	if err != nil {
		return nil, err
	}
	spec.name = options.Name
	spec.fallback = options.FallbackMarshalerUnmarshaler
	p.modelTypeToSpec[typ] = spec
	p.modelNameToSpec[options.Name] = spec

	collection := &Collection{
		spec:  spec,
		pool:  p,
		index: options.Index,
	}
	addCollection(collection)
	return collection, nil
}

// Name returns the name for the given collection. The name is a unique string
// identifier to use for the collection in redis. All models in this collection
// that are saved in the database will use the collection name as a prefix.
func (c *Collection) Name() string {
	return c.spec.name
}

// addCollection adds the given spec to the list of collections iff it has not
// already been added.
func addCollection(collection *Collection) {
	for e := collections.Front(); e != nil; e = e.Next() {
		otherCollection := e.Value.(*Collection)
		if collection.spec.typ == otherCollection.spec.typ {
			// The Collection was already added to the list. No need to do
			// anything.
			return
		}
	}
	collections.PushFront(collection)
}

// getCollectionForModel returns the Collection corresponding to the type of
// model.
func getCollectionForModel(model Model) (*Collection, error) {
	typ := reflect.TypeOf(model)
	for e := collections.Front(); e != nil; e = e.Next() {
		col := e.Value.(*Collection)
		if col.spec.typ == typ {
			return col, nil
		}
	}
	return nil, fmt.Errorf("Could not find Collection for type %T", model)
}

func (p *Pool) typeIsRegistered(typ reflect.Type) bool {
	_, found := p.modelTypeToSpec[typ]
	return found
}

func (p *Pool) nameIsRegistered(name string) bool {
	_, found := p.modelNameToSpec[name]
	return found
}

// ModelKey returns the key that identifies a hash in the database
// which contains all the fields of the model corresponding to the given
// id. If id is an empty string, it will return an empty string.
func (c *Collection) ModelKey(id string) string {
	if id == "" {
		return ""
	}
	// c.spec.modelKey(id) will only return an error if id was an empty string.
	// Since we already ruled that out with the check above, we can safely ignore
	// the error return value here.
	key, _ := c.spec.modelKey(id)
	return key
}

// IndexKey returns the key that identifies a set in the database that
// stores all the ids for models in the given collection.
func (c *Collection) IndexKey() string {
	return c.spec.indexKey()
}

// FieldIndexKey returns the key for the sorted set used to index the field
// identified by fieldName. It returns an error if fieldName does not identify a
// field in the spec or if the field it identifies is not an indexed field.
func (c *Collection) FieldIndexKey(fieldName string) (string, error) {
	return c.spec.fieldIndexKey(fieldName)
}

// FieldNames returns all the field names for the Collection. The order is
// always the same and is used internally by Zoom to determine the order of
// fields in Redis commands such as HMGET.
func (c *Collection) FieldNames() []string {
	return c.spec.fieldNames()
}

// FieldRedisNames returns all the Redis names for the fields of the Collection.
// For example, if a Collection was created with a model type that includes
// custom field names via the `redis` struct tag, those names will be returned.
// The order is always the same and is used internally by Zoom to determine the
// order of fields in Redis commands such as HMGET.
func (c *Collection) FieldRedisNames() []string {
	return c.spec.fieldRedisNames()
}

// newNilCollectionError returns an error with a message describing that
// methodName was called on a nil collection.
func newNilCollectionError(methodName string) error {
	return fmt.Errorf("zoom: Called %s on nil collection. You must initialize the collection with Pool.NewCollection", methodName)
}

// newUnindexedCollectionError returns an error with a message describing that
// methodName was called on a collection that was not indexed. Certain methods
// can only be called on indexed collections.
func newUnindexedCollectionError(methodName string) error {
	return fmt.Errorf("zoom: %s only works for indexed collections. To index the collection, set the Index property to true in CollectionOptions when calling Pool.NewCollection", methodName)
}

// Save writes a model (a struct which satisfies the Model interface) to the
// redis database. Save returns an error if the type of model does not match the
// registered Collection. To make a struct satisfy the Model interface, you can
// embed zoom.RandomID, which will generate pseudo-random ids for each model.
func (c *Collection) Save(model Model) error {
	t := c.pool.NewTransaction()
	t.Save(c, model)
	if err := t.Exec(); err != nil {
		return err
	}
	return nil
}

// Save writes a model (a struct which satisfies the Model interface) to the
// redis database inside an existing transaction. save will set the err property
// of the transaction if the type of model does not match the registered
// Collection, which will cause exec to fail immediately and return the error.
// To make a struct satisfy the Model interface, you can embed zoom.RandomID,
// which will generate pseudo-random ids for each model. Any errors encountered
// will be added to the transaction and returned as an error when the
// transaction is executed.
func (t *Transaction) Save(c *Collection, model Model) {
	if c == nil {
		t.setError(newNilCollectionError("Save"))
		return
	}
	if err := c.checkModelType(model); err != nil {
		t.setError(fmt.Errorf("zoom: Error in Save or Transaction.Save: %s", err.Error()))
		return
	}
	// Create a modelRef and start a transaction
	mr := &modelRef{
		collection: c,
		model:      model,
		spec:       c.spec,
	}
	// Save indexes
	// This must happen first, because it relies on reading the old field values
	// from the hash for string indexes (if any)
	t.saveFieldIndexes(mr)
	// Save the model fields in a hash in the database
	hashArgs, err := mr.mainHashArgs()
	if err != nil {
		t.setError(err)
	}
	if len(hashArgs) > 1 {
		// Only save the main hash if there are any fields
		// The first element in hashArgs is the model key,
		// so there are fields if the length is greater than
		// 1.
		t.Command("HMSET", hashArgs, nil)
	}
	// Add the model id to the set of all models for this collection
	if c.index {
		t.Command("SADD", redis.Args{c.IndexKey(), model.ModelID()}, nil)
	}
}

// saveFieldIndexes adds commands to the transaction for saving the indexes
// for all indexed fields.
func (t *Transaction) saveFieldIndexes(mr *modelRef) {
	t.saveFieldIndexesForFields(mr.spec.fieldNames(), mr)
}

// saveFieldIndexesForFields works like saveFieldIndexes, but only saves the
// indexes for the given fieldNames.
func (t *Transaction) saveFieldIndexesForFields(fieldNames []string, mr *modelRef) {
	for _, fs := range mr.spec.fields {
		// Skip fields whose names do not appear in fieldNames.
		if !stringSliceContains(fieldNames, fs.name) {
			continue
		}
		switch fs.indexKind {
		case noIndex:
			continue
		case numericIndex:
			t.saveNumericIndex(mr, fs)
		case booleanIndex:
			t.saveBooleanIndex(mr, fs)
		case stringIndex:
			t.saveStringIndex(mr, fs)
		}
	}
}

// saveNumericIndex adds commands to the transaction for saving a numeric
// index on the given field.
func (t *Transaction) saveNumericIndex(mr *modelRef, fs *fieldSpec) {
	fieldValue := mr.fieldValue(fs.name)
	if fieldValue.Kind() == reflect.Ptr && fieldValue.IsNil() {
		return
	}
	score := numericScore(fieldValue)
	indexKey, err := mr.spec.fieldIndexKey(fs.name)
	if err != nil {
		t.setError(err)
	}
	t.Command("ZADD", redis.Args{indexKey, score, mr.model.ModelID()}, nil)
}

// saveBooleanIndex adds commands to the transaction for saving a boolean
// index on the given field.
func (t *Transaction) saveBooleanIndex(mr *modelRef, fs *fieldSpec) {
	fieldValue := mr.fieldValue(fs.name)
	if fieldValue.Kind() == reflect.Ptr && fieldValue.IsNil() {
		return
	}
	score := boolScore(fieldValue)
	indexKey, err := mr.spec.fieldIndexKey(fs.name)
	if err != nil {
		t.setError(err)
	}
	t.Command("ZADD", redis.Args{indexKey, score, mr.model.ModelID()}, nil)
}

// saveStringIndex adds commands to the transaction for saving a string
// index on the given field. This includes removing the old index (if any).
func (t *Transaction) saveStringIndex(mr *modelRef, fs *fieldSpec) {
	// Remove the old index (if any)
	t.deleteStringIndex(mr.spec.name, mr.model.ModelID(), fs.redisName)
	fieldValue := mr.fieldValue(fs.name)
	for fieldValue.Kind() == reflect.Ptr {
		if fieldValue.IsNil() {
			return
		}
		fieldValue = fieldValue.Elem()
	}
	member := fieldValue.String() + nullString + mr.model.ModelID()
	indexKey, err := mr.spec.fieldIndexKey(fs.name)
	if err != nil {
		t.setError(err)
	}
	t.Command("ZADD", redis.Args{indexKey, 0, member}, nil)
}

// SaveFields saves only the given fields of the model. SaveFields uses
// "last write wins" semantics. If another caller updates the the same fields
// concurrently, your updates may be overwritten. It will return an error if
// the type of model does not match the registered Collection, or if any of
// the given fieldNames are not found in the registered Collection. If
// SaveFields is called on a model that has not yet been saved, it will not
// return an error. Instead, only the given fields will be saved in the
// database.
func (c *Collection) SaveFields(fieldNames []string, model Model) error {
	t := c.pool.NewTransaction()
	t.SaveFields(c, fieldNames, model)
	if err := t.Exec(); err != nil {
		return err
	}
	return nil
}

// SaveFields saves only the given fields of the model inside an existing
// transaction. SaveFields will set the err property of the transaction if the
// type of model does not match the registered Collection, or if any of the
// given fieldNames are not found in the model type. In either case, the
// transaction will return the error when you call Exec. SaveFields uses "last
// write wins" semantics. If another caller updates the the same fields
// concurrently, your updates may be overwritten. If SaveFields is called on a
// model that has not yet been saved, it will not return an error. Instead, only
// the given fields will be saved in the database.
func (t *Transaction) SaveFields(c *Collection, fieldNames []string, model Model) {
	// Check the model type
	if err := c.checkModelType(model); err != nil {
		t.setError(fmt.Errorf("zoom: Error in SaveFields or Transaction.SaveFields: %s", err.Error()))
		return
	}
	// Check the given field names
	for _, fieldName := range fieldNames {
		if !stringSliceContains(c.spec.fieldNames(), fieldName) {
			t.setError(fmt.Errorf("zoom: Error in SaveFields or Transaction.SaveFields: Collection %s does not have field named %s", c.Name(), fieldName))
			return
		}
	}
	// Create a modelRef and start a transaction
	mr := &modelRef{
		collection: c,
		model:      model,
		spec:       c.spec,
	}
	// Update indexes
	// This must happen first, because it relies on reading the old field values
	// from the hash for string indexes (if any)
	t.saveFieldIndexesForFields(fieldNames, mr)
	// Get the main hash args.
	hashArgs, err := mr.mainHashArgsForFields(fieldNames)
	if err != nil {
		t.setError(err)
	}
	//
	if len(hashArgs) > 1 {
		// Only save the main hash if there are any fields
		// The first element in hashArgs is the model key,
		// so there are fields if the length is greater than
		// 1.
		t.Command("HMSET", hashArgs, nil)
	}
	// Add the model id to the set of all models for this collection
	if c.index {
		t.Command("SADD", redis.Args{c.IndexKey(), model.ModelID()}, nil)
	}
}

// Find retrieves a model with the given id from redis and scans its values
// into model. model should be a pointer to a struct of a registered type
// corresponding to the Collection. Find will mutate the struct, filling in its
// fields and overwriting any previous values. It returns an error if a model
// with the given id does not exist, if the given model was the wrong type, or
// if there was a problem connecting to the database.
func (c *Collection) Find(id string, model Model) error {
	t := c.pool.NewTransaction()
	t.Find(c, id, model)
	if err := t.Exec(); err != nil {
		return err
	}
	return nil
}

// Find retrieves a model with the given id from redis and scans its values
// into model in an existing transaction. model should be a pointer to a struct
// of a registered type corresponding to the Collection. find will mutate the struct,
// filling in its fields and overwriting any previous values. Any errors encountered
// will be added to the transaction and returned as an error when the transaction is
// executed.
func (t *Transaction) Find(c *Collection, id string, model Model) {
	if c == nil {
		t.setError(newNilCollectionError("Find"))
		return
	}
	if err := c.checkModelType(model); err != nil {
		t.setError(fmt.Errorf("zoom: Error in Find or Transaction.Find: %s", err.Error()))
		return
	}
	model.SetModelID(id)
	mr := &modelRef{
		collection: c,
		model:      model,
		spec:       c.spec,
	}
	// Check if the model actually exists
	t.Command("EXISTS", redis.Args{mr.key()}, newModelExistsHandler(c, id))
	// Get the fields from the main hash for this model
	args := redis.Args{mr.key()}
	for _, fieldName := range mr.spec.fieldRedisNames() {
		args = append(args, fieldName)
	}
	t.Command("HMGET", args, newScanModelRefHandler(mr.spec.fieldNames(), mr))
}

// FindFields is like Find but finds and sets only the specified fields. Any
// fields of the model which are not in the given fieldNames are not mutated.
// FindFields will return an error if any of the given fieldNames are not found
// in the model type.
func (c *Collection) FindFields(id string, fieldNames []string, model Model) error {
	t := c.pool.NewTransaction()
	t.FindFields(c, id, fieldNames, model)
	if err := t.Exec(); err != nil {
		return err
	}
	return nil
}

// FindFields is like Find but finds and sets only the specified fields. Any
// fields of the model which are not in the given fieldNames are not mutated.
// FindFields will return an error if any of the given fieldNames are not found
// in the model type.
func (t *Transaction) FindFields(c *Collection, id string, fieldNames []string, model Model) {
	if err := c.checkModelType(model); err != nil {
		t.setError(fmt.Errorf("zoom: Error in FindFields or Transaction.FindFields: %s", err.Error()))
		return
	}
	// Set the model id and create a modelRef
	model.SetModelID(id)
	mr := &modelRef{
		collection: c,
		spec:       c.spec,
		model:      model,
	}
	// Check the given field names and append the corresponding redis field names
	// to args.
	args := redis.Args{mr.key()}
	for _, fieldName := range fieldNames {
		if !stringSliceContains(c.spec.fieldNames(), fieldName) {
			t.setError(fmt.Errorf("zoom: Error in FindFields or Transaction.FindFields: Collection %s does not have field named %s", c.Name(), fieldName))
			return
		}
		// args is an array of arguments passed to the HMGET command. We want to
		// use the redis names corresponding to each field name. The redis names
		// may be customized via struct tags.
		args = append(args, c.spec.fieldsByName[fieldName].redisName)
	}
	// Check if the model actually exists.
	t.Command("EXISTS", redis.Args{mr.key()}, newModelExistsHandler(c, id))
	// Get the fields from the main hash for this model
	t.Command("HMGET", args, newScanModelRefHandler(fieldNames, mr))
}

// FindAll finds all the models of the given type. It executes the commands needed
// to retrieve the models in a single transaction. See http://redis.io/topics/transactions.
// models must be a pointer to a slice of models with a type corresponding to the Collection.
// FindAll will grow or shrink the models slice as needed and if any of the models in the
// models slice are nil, FindAll will use reflection to allocate memory for them.
// FindAll returns an error if models is the wrong type or if there was a problem connecting
// to the database.
func (c *Collection) FindAll(models interface{}) error {
	// Since this is somewhat type-unsafe, we need to verify that
	// models is the correct type
	t := c.pool.NewTransaction()
	t.FindAll(c, models)
	if err := t.Exec(); err != nil {
		return err
	}
	return nil
}

// FindAll finds all the models of the given type and scans the values of the models into
// models in an existing transaction. See http://redis.io/topics/transactions.
// models must be a pointer to a slice of models with a type corresponding to the Collection.
// findAll will grow the models slice as needed and if any of the models in the
// models slice are nil, FindAll will use reflection to allocate memory for them.
// Any errors encountered will be added to the transaction and returned as an error
// when the transaction is executed.
func (t *Transaction) FindAll(c *Collection, models interface{}) {
	if c == nil {
		t.setError(newNilCollectionError("FindAll"))
		return
	}
	if !c.index {
		t.setError(newUnindexedCollectionError("FindAll"))
		return
	}
	// Since this is somewhat type-unsafe, we need to verify that
	// models is the correct type
	if err := c.checkModelsType(models); err != nil {
		t.setError(fmt.Errorf("zoom: Error in FindAll or Transaction.FindAll: %s", err.Error()))
		return
	}
	sortArgs := c.spec.sortArgs(c.spec.indexKey(), c.spec.fieldRedisNames(), 0, 0, false)
	fieldNames := append(c.spec.fieldNames(), "-")
	t.Command("SORT", sortArgs, newScanModelsHandler(c.spec, fieldNames, models))
}

// Exists returns true if the collection has a model with the given id. It
// returns an error if there was a problem connecting to the database.
func (c *Collection) Exists(id string) (bool, error) {
	t := c.pool.NewTransaction()
	exists := false
	t.Exists(c, id, &exists)
	if err := t.Exec(); err != nil {
		return false, err
	}
	return exists, nil
}

// Exists sets the value of exists to true if a model exists in the given
// collection with the given id, and sets it to false otherwise. The first error
// encountered (if any) will be added to the transaction and returned when
// the transaction is executed.
func (t *Transaction) Exists(c *Collection, id string, exists *bool) {
	if c == nil {
		t.setError(newNilCollectionError("Exists"))
		return
	}
	t.Command("EXISTS", redis.Args{c.ModelKey(id)}, NewScanBoolHandler(exists))
}

// Count returns the number of models of the given type that exist in the database.
// It returns an error if there was a problem connecting to the database.
func (c *Collection) Count() (int, error) {
	t := c.pool.NewTransaction()
	count := 0
	t.Count(c, &count)
	if err := t.Exec(); err != nil {
		return 0, err
	}
	return count, nil
}

// Count counts the number of models of the given type in the database in an existing
// transaction. It sets the value of count to the number of models. Any errors
// encountered will be added to the transaction and returned as an error when the
// transaction is executed.
func (t *Transaction) Count(c *Collection, count *int) {
	if c == nil {
		t.setError(newNilCollectionError("Count"))
		return
	}
	if !c.index {
		t.setError(newUnindexedCollectionError("Count"))
		return
	}
	t.Command("SCARD", redis.Args{c.IndexKey()}, NewScanIntHandler(count))
}

// Delete removes the model with the given type and id from the database. It will
// not return an error if the model corresponding to the given id was not
// found in the database. Instead, it will return a boolean representing whether
// or not the model was found and deleted, and will only return an error
// if there was a problem connecting to the database.
func (c *Collection) Delete(id string) (bool, error) {
	t := c.pool.NewTransaction()
	deleted := false
	t.Delete(c, id, &deleted)
	if err := t.Exec(); err != nil {
		return deleted, err
	}
	return deleted, nil
}

// Delete removes a model with the given type and id in an existing transaction.
// deleted will be set to true iff the model was successfully deleted when the
// transaction is executed. If the no model with the given type and id existed,
// the value of deleted will be set to false. Any errors encountered will be
// added to the transaction and returned as an error when the transaction is
// executed. You may pass in nil for deleted if you do not care whether or not
// the model was deleted.
func (t *Transaction) Delete(c *Collection, id string, deleted *bool) {
	if c == nil {
		t.setError(newNilCollectionError("Delete"))
		return
	}
	// Delete any field indexes
	// This must happen first, because it relies on reading the old field values
	// from the hash for string indexes (if any)
	t.deleteFieldIndexes(c, id)
	var handler ReplyHandler
	if deleted == nil {
		handler = nil
	} else {
		handler = NewScanBoolHandler(deleted)
	}
	// Delete the main hash
	t.Command("DEL", redis.Args{c.Name() + ":" + id}, handler)
	// Remvoe the id from the index of all models for the given type
	t.Command("SREM", redis.Args{c.IndexKey(), id}, nil)
}

// deleteFieldIndexes adds commands to the transaction for deleting the field
// indexes for all indexed fields of the given model type.
func (t *Transaction) deleteFieldIndexes(c *Collection, id string) {
	for _, fs := range c.spec.fields {
		switch fs.indexKind {
		case noIndex:
			continue
		case numericIndex, booleanIndex:
			t.deleteNumericOrBooleanIndex(fs, c.spec, id)
		case stringIndex:
			// NOTE: this invokes a lua script which is defined in scripts/delete_string_index.lua
			t.deleteStringIndex(c.Name(), id, fs.redisName)
		}
	}
}

// deleteNumericOrBooleanIndex removes the model from a numeric or boolean index for the given
// field. I.e. it removes the model id from a sorted set.
func (t *Transaction) deleteNumericOrBooleanIndex(fs *fieldSpec, ms *modelSpec, modelID string) {
	indexKey, err := ms.fieldIndexKey(fs.name)
	if err != nil {
		t.setError(err)
	}
	t.Command("ZREM", redis.Args{indexKey, modelID}, nil)
}

// DeleteAll deletes all the models of the given type in a single transaction. See
// http://redis.io/topics/transactions. It returns the number of models deleted
// and an error if there was a problem connecting to the database.
func (c *Collection) DeleteAll() (int, error) {
	t := c.pool.NewTransaction()
	count := 0
	t.DeleteAll(c, &count)
	if err := t.Exec(); err != nil {
		return count, err
	}
	return count, nil
}

// DeleteAll delets all models for the given model type in an existing transaction.
// The value of count will be set to the number of models that were successfully deleted
// when the transaction is executed. Any errors encountered will be added to the transaction
// and returned as an error when the transaction is executed. You may pass in nil
// for count if you do not care about the number of models that were deleted.
func (t *Transaction) DeleteAll(c *Collection, count *int) {
	if c == nil {
		t.setError(newNilCollectionError("DeleteAll"))
		return
	}
	if !c.index {
		t.setError(newUnindexedCollectionError("DeleteAll"))
		return
	}
	var handler ReplyHandler
	if count == nil {
		handler = nil
	} else {
		handler = NewScanIntHandler(count)
	}
	t.DeleteModelsBySetIDs(c.IndexKey(), c.Name(), handler)
}

// checkModelType returns an error iff model is not of the registered type that
// corresponds to c.
func (c *Collection) checkModelType(model Model) error {
	return c.spec.checkModelType(model)
}

// checkModelsType returns an error iff models is not a pointer to a slice of models of the
// registered type that corresponds to the collection.
func (c *Collection) checkModelsType(models interface{}) error {
	return c.spec.checkModelsType(models)
}
