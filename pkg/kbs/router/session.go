/*
 * Copyright (C) 2020 Intel Corporation
 * SPDX-License-Identifier: BSD-3-Clause
 */
package router

import (
	"github.com/gorilla/mux"
	"github.com/intel-secl/intel-secl/v3/pkg/kbs/constants"
	"github.com/intel-secl/intel-secl/v3/pkg/kbs/controllers"
)

//setSessionRoutes registers routes to perform session management operations
func setSessionRoutes(router *mux.Router) *mux.Router {
	defaultLog.Trace("router/keys:setSessionRoutes() Entering")
	defer defaultLog.Trace("router/keys:setSessionRoutes() Leaving")

	sessionController := controllers.NewSessionController()

	router.Handle("/session",
		ErrorHandler(permissionsHandler(JsonResponseHandler(sessionController.Create),
			[]string{constants.SessionCreate}))).Methods("POST")

	return router
}