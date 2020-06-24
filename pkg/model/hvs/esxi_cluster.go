/*
 *  Copyright (C) 2020 Intel Corporation
 *  SPDX-License-Identifier: BSD-3-Clause
 */

package hvs

import "github.com/google/uuid"

type ESXiClusterCollection struct {
	ESXiCluster []*ESXiCluster `json:"esxi_clusters"`
}

type ESXiCluster struct {
	Id               uuid.UUID `json:"id,omitempty"`
	ConnectionString string    `json:"connection_string"`
	ClusterName      string    `json:"cluster_name"`
	//TODO : Add structure for host
}