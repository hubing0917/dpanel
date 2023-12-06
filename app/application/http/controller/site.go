package controller

import (
	"context"
	"errors"
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/donknap/dpanel/app/application/logic"
	"github.com/donknap/dpanel/common/accessor"
	"github.com/donknap/dpanel/common/dao"
	"github.com/donknap/dpanel/common/entity"
	"github.com/donknap/dpanel/common/function"
	"github.com/donknap/dpanel/common/service/docker"
	"github.com/gin-gonic/gin"
	"github.com/we7coreteam/w7-rangine-go/src/core/err_handler"
	"github.com/we7coreteam/w7-rangine-go/src/http/controller"
	"strings"
)

type Site struct {
	controller.Abstract
}

func (self Site) CreateByImage(http *gin.Context) {
	type ParamsValidate struct {
		SiteName string `form:"siteName" binding:"required"`
		SiteUrl  string `form:"siteUrl" binding:"required,url"`
		Image    string `json:"image" binding:"required"`
		Type     string `json:"type" binding:"required,oneof=system site"`
		accessor.SiteEnvOption
	}

	params := ParamsValidate{}
	if !self.Validate(http, &params) {
		return
	}

	if params.Ports != nil {
		var checkPorts []string
		for _, port := range params.Ports {
			checkPorts = append(checkPorts, port.Host)
		}
		if checkPorts != nil {
			sdk, err := docker.NewDockerClient()
			if err != nil {
				self.JsonResponseWithError(http, err, 500)
				return
			}
			item, _ := sdk.ContainerByField("publish", checkPorts...)
			if len(item) > 0 {
				self.JsonResponseWithError(http, errors.New("绑定的外部端口已经被占用，请更换其它端外部端口"), 500)
				return
			}
		}
	}

	if params.Type == logic.SITE_TYPE_SITE {
		query := dao.Site.Where(dao.Site.SiteURL.Eq(params.SiteUrl))
		site, _ := query.First()
		if site != nil {
			self.JsonResponseWithError(http, errors.New("站点域名已经绑定其它站，请更换域名"), 500)
		}
	}

	runParams := accessor.SiteEnvOption{
		Environment: params.Environment,
		Volumes:     params.Volumes,
		Ports:       params.Ports,
		Links:       params.Links,
		Image:       accessor.ImageItem{},
	}

	if params.Image != "" {
		imageArr := strings.Split(
			params.Image+":",
			":",
		)
		runParams.Image.Name = imageArr[0]
		runParams.Image.Version = imageArr[1]
	}

	// 如果是系统组件，域名相关配置可以去掉
	if params.Type == logic.SITE_TYPE_SYSTEM {
		params.SiteUrl = ""
	}
	siteUrlExt := &accessor.SiteUrlExtOption{}
	siteUrlExt.Url = append(siteUrlExt.Url, params.SiteUrl)

	siteRow := &entity.Site{
		SiteID:     "",
		SiteName:   params.SiteName,
		SiteURL:    params.SiteUrl,
		SiteURLExt: siteUrlExt,
		Env:        &runParams,
		Status:     logic.STATUS_STOP,
		ContainerInfo: &accessor.SiteContainerInfoOption{
			ID: "",
		},
		Type: logic.SiteTypeValue[params.Type],
	}
	err := dao.Site.Create(siteRow)
	if err != nil {
		self.JsonResponseWithError(http, err, 500)
		return
	}
	siteRow.SiteID = fmt.Sprintf("dpanel-%s-%d-%s", params.Type, siteRow.ID, function.GetRandomString(10))
	dao.Site.Where(dao.Site.ID.Eq(siteRow.ID)).Updates(siteRow)
	if err_handler.Found(err) {
		self.JsonResponseWithError(http, err, 500)
		return
	}
	task := logic.NewContainerTask()
	runTaskRow := &logic.CreateMessage{
		Name:      siteRow.SiteID,
		SiteId:    siteRow.ID,
		RunParams: &runParams,
	}
	task.QueueCreate <- runTaskRow
	if err != nil {
		self.JsonResponseWithError(http, err, 500)
		return
	}
	self.JsonResponseWithoutError(http, gin.H{"siteId": siteRow.ID})
	return
}

func (self Site) GetList(http *gin.Context) {
	type ParamsValidate struct {
		Page     int    `form:"page,default=1" binding:"omitempty,gt=0"`
		PageSize int    `form:"pageSize" binding:"omitempty"`
		SiteName string `form:"siteName" binding:"omitempty"`
		Sort     string `form:"sort,default=new" binding:"omitempty,oneof=hot new"`
		Type     string `form:"type" binding:"oneof=system site"`
		Status   int32  `json:"status" binding:"omitempty,oneof=10 20 30"`
	}

	params := ParamsValidate{}
	if !self.Validate(http, &params) {
		return
	}
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 {
		params.PageSize = 10
	}

	query := dao.Site.Order(dao.Site.ID.Desc())
	if params.Type != "" {
		query = query.Where(dao.Site.Type.Eq(logic.SiteTypeValue[params.Type]))
	}
	if params.Status != 0 {
		query = query.Where(dao.Site.Status.Eq(params.Status))
	}
	if params.SiteName != "" {
		query = query.Where(dao.Site.SiteName.Like("%" + params.SiteName + "%"))
	}
	list, total, _ := query.FindByPage((params.Page-1)*params.PageSize, params.PageSize)

	// 列表数据，如果容器状态有问题，需要再次更新状态
	if list != nil {
		for _, site := range list {
			// 有容器信息后，站点的状态跟随容器状态
			if site.Status > logic.STATUS_PROCESSING && site.ContainerInfo != nil {
				dao.Site.Where(dao.Site.ID.Eq(site.ID)).Update(dao.Site.Status, site.ContainerInfo.Status)
				site.Status = site.ContainerInfo.Status
			}
		}
	}
	self.JsonResponseWithoutError(http, gin.H{
		"total": total,
		"page":  params.Page,
		"list":  list,
	})
	return
}

func (self Site) GetDetail(http *gin.Context) {
	type ParamsValidate struct {
		Id int32 `form:"id" binding:"required"`
	}

	params := ParamsValidate{}
	if !self.Validate(http, &params) {
		return
	}

	siteRow, _ := dao.Site.Where(dao.Site.ID.Eq(params.Id)).First()
	if siteRow == nil {
		self.JsonResponseWithError(http, errors.New("站点不存在"), 500)
		return
	}
	// 更新容器信息
	self.JsonResponseWithoutError(http, siteRow)
	return
}

func (self Site) ReDeploy(http *gin.Context) {
	type ParamsValidate struct {
		Id    int32  `form:"id" binding:"required"`
		Image string `form:"image" binding:"omitempty"`
	}

	params := ParamsValidate{}
	if !self.Validate(http, &params) {
		return
	}

	siteRow, _ := dao.Site.Where(dao.Site.ID.Eq(params.Id)).First()
	if siteRow == nil {
		self.JsonResponseWithError(http, errors.New("站点不存在"), 500)
		return
	}
	sdk, err := docker.NewDockerClient()
	if err != nil {
		self.JsonResponseWithError(http, err, 500)
		return
	}
	if siteRow.ContainerInfo != nil && siteRow.ContainerInfo.ID != "" {
		ctx := context.Background()
		sdk.Client.ContainerStop(ctx, siteRow.ContainerInfo.ID, container.StopOptions{})
		err = sdk.Client.ContainerRemove(ctx, siteRow.ContainerInfo.ID, types.ContainerRemoveOptions{})
		if err != nil {
			self.JsonResponseWithError(http, err, 500)
			return
		}

	}

	runParams := &accessor.SiteEnvOption{
		Environment: siteRow.Env.Environment,
		Volumes:     siteRow.Env.Volumes,
		Ports:       siteRow.Env.Ports,
		Links:       siteRow.Env.Links,
		Image:       siteRow.Env.Image,
	}
	if params.Image != "" {
		imageArr := strings.Split(
			params.Image+":",
			":",
		)
		runParams.Image.Name = imageArr[0]
		runParams.Image.Version = imageArr[1]
	}
	task := logic.NewContainerTask()
	runTaskRow := &logic.CreateMessage{
		Name:      siteRow.SiteID,
		SiteId:    siteRow.ID,
		RunParams: runParams,
	}
	task.QueueCreate <- runTaskRow
	if err != nil {
		self.JsonResponseWithError(http, err, 500)
		return
	}
	self.JsonResponseWithoutError(http, gin.H{"siteId": siteRow.ID})

	return
}

func (self Site) Delete(http *gin.Context) {
	type ParamsValidate struct {
		Id           int32 `form:"id" binding:"required"`
		DeleteImage  bool  `form:"deleteImage" binding:"omitempty"`
		DeleteVolume bool  `form:"deleteVolume" binding:"omitempty"`
		DeleteLink   bool  `form:"deleteLink" binding:"omitempty"`
	}
	params := ParamsValidate{}
	if !self.Validate(http, &params) {
		return
	}

	siteRow, _ := dao.Site.Where(dao.Site.ID.Eq(params.Id)).First()
	if siteRow == nil {
		self.JsonResponseWithError(http, errors.New("站点不存在"), 500)
		return
	}
	var err error
	sdk, err := docker.NewDockerClient()
	if err != nil {
		self.JsonResponseWithError(http, err, 500)
		return
	}
	if siteRow.ContainerInfo != nil && siteRow.ContainerInfo.ID != "" {
		ctx := context.Background()
		sdk.Client.ContainerStop(ctx, siteRow.ContainerInfo.ID, container.StopOptions{})
		err = sdk.Client.ContainerRemove(context.Background(), siteRow.ContainerInfo.ID, types.ContainerRemoveOptions{
			RemoveVolumes: params.DeleteVolume,
			RemoveLinks:   params.DeleteLink,
		})
		sdk.Client.ImageRemove(ctx, siteRow.ContainerInfo.Info.ImageID, types.ImageRemoveOptions{})
	}
	if err != nil {
		self.JsonResponseWithError(http, err, 500)
		return
	}
	dao.Site.Where(dao.Site.ID.Eq(params.Id)).Delete()
	dao.Task.Where(dao.Task.TaskID.Eq(params.Id)).Delete()

	self.JsonResponseWithoutError(http, gin.H{
		"siteId": params.Id,
	})
	return
}
