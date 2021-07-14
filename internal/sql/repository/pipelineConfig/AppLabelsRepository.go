/*
 * Copyright (c) 2020 Devtron Labs
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

package pipelineConfig

import (
	"fmt"
	"github.com/devtron-labs/devtron/internal/sql/models"
	"github.com/go-pg/pg"
)

type AppLabel struct {
	tableName struct{} `sql:"app_label" pg:",discard_unknown_columns"`
	Id        int      `sql:"id,pk"`
	Key       string   `sql:"key,notnull"`
	Value     string   `sql:"value,notnull"`
	AppId     int      `sql:"app_id"`
	App       App
	models.AuditLog
}

type AppLabelRepository interface {
	Create(model *AppLabel) (*AppLabel, error)
	Update(model *AppLabel) (*AppLabel, error)
	FindById(id int) (*AppLabel, error)
	FindAll() ([]*AppLabel, error)
	FindByLabelKey(key string) ([]*AppLabel, error)
	FindByAppIdAndKeyAndValue(appId int, key string, value string) (*AppLabel, error)
	FindByLabels(labels []string) ([]*AppLabel, error)
	FindAllByAppId(appId int) ([]*AppLabel, error)
}

type AppLabelRepositoryImpl struct {
	dbConnection *pg.DB
}

func NewAppLabelRepositoryImpl(dbConnection *pg.DB) *AppLabelRepositoryImpl {
	return &AppLabelRepositoryImpl{dbConnection: dbConnection}
}

func (impl AppLabelRepositoryImpl) Create(model *AppLabel) (*AppLabel, error) {
	err := impl.dbConnection.Insert(model)
	if err != nil {
		return model, err
	}
	return model, nil
}
func (impl AppLabelRepositoryImpl) Update(model *AppLabel) (*AppLabel, error) {
	err := impl.dbConnection.Update(model)
	if err != nil {
		return model, err
	}

	return model, nil
}
func (impl AppLabelRepositoryImpl) FindById(id int) (*AppLabel, error) {
	var model AppLabel
	err := impl.dbConnection.Model(&model).Where("id = ?", id).Order("id desc").Limit(1).Select()
	return &model, err
}
func (impl AppLabelRepositoryImpl) FindAll() ([]*AppLabel, error) {
	var userModel []*AppLabel
	err := impl.dbConnection.Model(&userModel).Order("updated_on desc").Select()
	return userModel, err
}
func (impl AppLabelRepositoryImpl) FindByLabelKey(key string) ([]*AppLabel, error) {
	var model []*AppLabel
	err := impl.dbConnection.Model(&model).Where("key = ?", key).Select()
	return model, err
}
func (impl AppLabelRepositoryImpl) FindByAppIdAndKeyAndValue(appId int, key string, value string) (*AppLabel, error) {
	var model *AppLabel
	err := impl.dbConnection.Model(&model).Where("appId = ?", appId).
		Where("key = ?", key).Where("value = ?", value).Select()
	return model, err
}

func (impl AppLabelRepositoryImpl) FindByLabels(labels []string) ([]*AppLabel, error) {
	if len(labels) == 0 {
		return nil, fmt.Errorf("no labels provided for search")
	}
	var models []*AppLabel
	err := impl.dbConnection.Model(&models).Where("labels in (?)", pg.In(labels)).Select()
	return models, err
}

func (impl AppLabelRepositoryImpl) FindAllByAppId(appId int) ([]*AppLabel, error) {
	var models []*AppLabel
	err := impl.dbConnection.Model(&models).Where("app_id=?", appId).Select()
	return models, err
}
