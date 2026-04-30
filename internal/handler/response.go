package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	CodeSuccess       = 0
	CodeBadRequest    = 40000
	CodeUnauthorized  = 40100
	CodeForbidden     = 40300
	CodeNotFound      = 40400
	CodeConflict      = 40900
	CodeInternalError = 50000
)

// response is the unified API response envelope.
type response struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data any    `json:"data"`
}

func sendOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, response{Code: CodeSuccess, Msg: "ok", Data: data})
}

func sendCreated(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, response{Code: CodeSuccess, Msg: "created", Data: data})
}

func sendFail(c *gin.Context, httpStatus int, code int, msg string) {
	c.JSON(httpStatus, response{Code: code, Msg: msg, Data: nil})
}

func sendBindErr(c *gin.Context, err error) {
	sendFail(c, http.StatusBadRequest, CodeBadRequest, err.Error())
}
