/*
 * Copyright (C) 2020 Intel Corporation
 * SPDX-License-Identifier: BSD-3-Clause
 */
package types

import (
	"encoding/xml"
	cf "github.com/intel-secl/intel-secl/v3/pkg/lib/flavor/common"
	"github.com/intel-secl/intel-secl/v3/pkg/lib/flavor/constants"
	cm "github.com/intel-secl/intel-secl/v3/pkg/lib/flavor/model"
	"github.com/intel-secl/intel-secl/v3/pkg/lib/flavor/util"
	hcConstants "github.com/intel-secl/intel-secl/v3/pkg/lib/host-connector/constants"
	hcTypes "github.com/intel-secl/intel-secl/v3/pkg/lib/host-connector/types"
	taModel "github.com/intel-secl/intel-secl/v3/pkg/model/ta"
	"github.com/pkg/errors"
	"strings"
)

/**
 *
 * @author mullas
 */

// LinuxPlatformFlavor is used to generate various Flavors for a Intel-based Linux host
type LinuxPlatformFlavor struct {
	HostManifest   *hcTypes.HostManifest        `json:"host_manifest"`
	HostInfo       *taModel.HostInfo            `json:"host_info"`
	TagCertificate *cm.X509AttributeCertificate `json:"tag_certificate"`
}

var (
	// This is a map of platform specific modules.
	// The map value (int) is not relevant, just use the map key for efficient lookups.
	platformModules = map[string]int{
		"LCP_DETAILS_HASH":     0,
		"BIOSAC_REG_DATA":      0,
		"OSSINITDATA_CAP_HASH": 0,
		"STM_HASH":             0,
		"MLE_HASH":             0,
		"NV_INFO_HASH":         0,
		"tb_policy":            0,
		"CPU_SCRTM_STAT":       0,
		"HASH_START":           0,
		"SINIT_PUBKEY_HASH":    0,
		"LCP_AUTHORITIES_HASH": 0,
		"EVTYPE_KM_HASH":       0,
		"EVTYPE_BPM_HASH":      0,
		"EVTYPE_KM_INFO_HASH":  0,
		"EVTYPE_BPM_INFO_HASH": 0,
		"EVTYPE_BOOT_POL_HASH": 0,
	}

	// map of os specific modules
	osModules = map[string]int{
		"vmlinuz": 0,
	}

	// map of host specific modules
	hostUniqueModules = map[string]int{
		"initrd":           0,
		"LCP_CONTROL_HASH": 0,
	}

	suefiPcrList = []int{0, 2, 3, 4, 6, 7}
	tbootPcrList = []int{17, 18}
)

var pfutil util.PlatformFlavorUtil
var sfutil util.SoftwareFlavorUtil

// NewLinuxPlatformFlavor returns an instance of LinuxPlatformFlavor
func NewLinuxPlatformFlavor(hostReport *hcTypes.HostManifest, tagCertificate *cm.X509AttributeCertificate) PlatformFlavor {
	log.Trace("flavor/types/linux_platform_flavor:NewLinuxPlatformFlavor() Entering")
	defer log.Trace("flavor/types/linux_platform_flavor:NewLinuxPlatformFlavor() Leaving")

	return LinuxPlatformFlavor{
		HostManifest:   hostReport,
		HostInfo:       &hostReport.HostInfo,
		TagCertificate: tagCertificate,
	}
}

// GetFlavorPartRaw extracts the details of the flavor part requested by the
// caller from the host report used during the creation of the PlatformFlavor instance
func (rhelpf LinuxPlatformFlavor) GetFlavorPartRaw(name cf.FlavorPart) ([]cm.Flavor, error) {
	log.Trace("flavor/types/linux_platform_flavor:GetFlavorPartRaw() Entering")
	defer log.Trace("flavor/types/linux_platform_flavor:GetFlavorPartRaw() Leaving")

	switch name {
	case cf.FlavorPartPlatform:
		return rhelpf.getPlatformFlavor()
	case cf.FlavorPartOs:
		return rhelpf.getOsFlavor()
	case cf.FlavorPartAssetTag:
		return rhelpf.getAssetTagFlavor()
	case cf.FlavorPartHostUnique:
		return rhelpf.getHostUniqueFlavor()
	case cf.FlavorPartSoftware:
		return rhelpf.getDefaultSoftwareFlavor()
	}
	return nil, cf.UNKNOWN_FLAVOR_PART()
}

// GetFlavorPartNames retrieves the list of flavor parts that can be obtained using the GetFlavorPartRaw function
func (rhelpf LinuxPlatformFlavor) GetFlavorPartNames() ([]cf.FlavorPart, error) {
	log.Trace("flavor/types/linux_platform_flavor:GetFlavorPartNames() Entering")
	defer log.Trace("flavor/types/linux_platform_flavor:GetFlavorPartNames() Leaving")

	flavorPartList := []cf.FlavorPart{cf.FlavorPartPlatform, cf.FlavorPartOs, cf.FlavorPartHostUnique, cf.FlavorPartSoftware}

	// For each of the flavor parts, check what PCRs are required and if those required PCRs are present in the host report.
	for i := 0; i < len(flavorPartList); i++ {
		flavorPart := flavorPartList[i]
		pcrList := rhelpf.getPcrList(flavorPart)
		pcrExists := pfutil.PcrExists(rhelpf.HostManifest.PcrManifest, pcrList)
		if !pcrExists {
			// remove the non-existent FlavorPart from list
			flavorPartList = append(flavorPartList[:i], flavorPartList[i+1:]...)
		}
	}

	// Check if the AssetTag flavor part is present by checking if tagCertificate is present
	if rhelpf.TagCertificate != nil {
		flavorPartList = append(flavorPartList, cf.FlavorPartAssetTag)
	}
	return flavorPartList, nil
}

// GetPcrList Helper function to calculate the list of PCRs for the flavor part specified based
// on the version of the TPM hardware.
func (rhelpf LinuxPlatformFlavor) getPcrList(flavorPart cf.FlavorPart) []int {
	log.Trace("flavor/types/linux_platform_flavor:getPcrList() Entering")
	defer log.Trace("flavor/types/linux_platform_flavor:getPcrList() Leaving")

	var pcrSet = make(map[int]bool)
	var pcrs []int
	var isTboot bool

	hostInfo := *rhelpf.HostInfo

	isTboot = hostInfo.TbootInstalled

	switch flavorPart {
	case cf.FlavorPartPlatform:
		pcrSet[0] = true
		// check if CBNT is supported
		if isCbntMeasureProfile(hostInfo.HardwareFeatures.CBNT) {
			log.Debug("flavor/types/linux_platform_flavor:getPcrList() PlatformFlavor - platform supports CBNT")
			pcrSet[7] = true
		}

		// check if SUEFI is supported
		if hostInfo.HardwareFeatures.SUEFI != nil {
			if hostInfo.HardwareFeatures.SUEFI.Enabled {
				for _, pcrx := range suefiPcrList {
					log.Debug("flavor/types/linux_platform_flavor:getPcrList() PlatformFlavor - platform supports SUEFI")
					pcrSet[pcrx] = true
				}
			}
		}

		// check if TBOOT is installed
		if isTboot {
			for _, pcrx := range tbootPcrList {
				log.Debug("flavor/types/linux_platform_flavor:getPcrList() PlatformFlavor - TBOOT is installed")
				pcrSet[pcrx] = true
			}
		}
	case cf.FlavorPartOs:
		// check if TBOOT is installed
		if isTboot {
			log.Debug("flavor/types/linux_platform_flavor:getPcrList() OSFlavor - TBOOT is installed")
			pcrSet[17] = true
		}

	case cf.FlavorPartHostUnique:
		// check if TBOOT is installed
		if isTboot {
			for _, pcrx := range tbootPcrList {
				log.Debug("flavor/types/linux_platform_flavor:getPcrList() HostUniqueFlavor - TBOOT is installed")
				pcrSet[pcrx] = true
			}
		}
	case cf.FlavorPartSoftware:
		pcrSet[15] = true
	}

	// convert set back to list
	for k := range pcrSet {
		pcrs = append(pcrs, k)
	}
	log.Debugf("flavor/types/linux_platform_flavor:getPcrList() PCRList %v", pcrs)
	return pcrs
}

func isCbntMeasureProfile(cbnt *taModel.CBNT) bool {
	log.Trace("flavor/types/linux_platform_flavor:isCbntMeasureProfile() Entering")
	defer log.Trace("flavor/types/linux_platform_flavor:isCbntMeasureProfile() Leaving")

	if cbnt != nil {
		return cbnt.Enabled && cbnt.Meta.Profile == cf.BootGuardProfile5().Name
	}
	return false
}

// eventLogRequired Helper function to determine if the event log associated with the PCR
// should be included in the flavor for the specified flavor part
func (rhelpf LinuxPlatformFlavor) eventLogRequired(flavorPartName cf.FlavorPart) bool {
	log.Trace("flavor/types/linux_platform_flavor:eventLogRequired() Entering")
	defer log.Trace("flavor/types/linux_platform_flavor:eventLogRequired() Leaving")

	// defaults to false
	var eventLogRequired bool

	switch flavorPartName {
	case cf.FlavorPartPlatform:
		eventLogRequired = true
	case cf.FlavorPartOs:
		eventLogRequired = true
	case cf.FlavorPartHostUnique:
		eventLogRequired = true
	case cf.FlavorPartSoftware:
		eventLogRequired = true
	}
	return eventLogRequired
}

// getPlatformFlavor returns a json document having all the good known PCR values and
// corresponding event logs that can be used for evaluating the PLATFORM trust of a host
func (rhelpf LinuxPlatformFlavor) getPlatformFlavor() ([]cm.Flavor, error) {
	log.Trace("flavor/types/linux_platform_flavor:getPlatformFlavor() Entering")
	defer log.Trace("flavor/types/linux_platform_flavor:getPlatformFlavor() Leaving")

	var errorMessage = "Error during creation of PLATFORM flavor"
	var platformPcrs = rhelpf.getPcrList(cf.FlavorPartPlatform)
	var includeEventLog = rhelpf.eventLogRequired(cf.FlavorPartPlatform)
	var allPcrDetails = pfutil.GetPcrDetails(
		rhelpf.HostManifest.PcrManifest, platformPcrs, includeEventLog)
	var filteredPcrDetails = pfutil.IncludeModulesToEventLog(
		allPcrDetails, platformModules)

	newMeta, err := pfutil.GetMetaSectionDetails(rhelpf.HostInfo, rhelpf.TagCertificate, "", cf.FlavorPartPlatform,
		hcConstants.VendorIntel)
	if err != nil {
		return nil, errors.Wrap(err, errorMessage+" - failure in Meta section details")
	}
	log.Debugf("flavor/types/linux_platform_flavor:getPlatformFlavor() New Meta Section: %v", *newMeta)

	newBios := pfutil.GetBiosSectionDetails(rhelpf.HostInfo)
	if newBios == nil {
		return nil, errors.Errorf(errorMessage + " - failure in Bios section details")
	}
	log.Debugf("flavor/types/linux_platform_flavor:getPlatformFlavor() New Bios Section: %v", *newBios)

	newHW := pfutil.GetHardwareSectionDetails(rhelpf.HostManifest)
	if newHW == nil {
		return nil, errors.Errorf(errorMessage + " - failure in Hardware section details")
	}
	log.Debugf("flavor/types/linux_platform_flavor:getPlatformFlavor() New Hardware Section: %v", *newHW)

	// Assemble the Platform Flavor
	platformFlavor := cm.NewFlavor(newMeta, newBios, newHW, filteredPcrDetails, nil, nil)

	log.Debugf("flavor/types/linux_platform_flavor:getPlatformFlavor()  New PlatformFlavor: %v", platformFlavor)

	return []cm.Flavor{*platformFlavor}, nil
}

// getOsFlavor Returns a json document having all the good known PCR values and
// corresponding event logs that can be used for evaluating the OS Trust of a host
func (rhelpf LinuxPlatformFlavor) getOsFlavor() ([]cm.Flavor, error) {
	log.Trace("flavor/types/linux_platform_flavor:getOsFlavor() Entering")
	defer log.Trace("flavor/types/linux_platform_flavor:getOsFlavor() Leaving")

	var errorMessage = "Error during creation of OS flavor"
	var err error
	var osPcrs = rhelpf.getPcrList(cf.FlavorPartOs)
	var includeEventLog = rhelpf.eventLogRequired(cf.FlavorPartOs)
	var allPcrDetails = pfutil.GetPcrDetails(
		rhelpf.HostManifest.PcrManifest, osPcrs, includeEventLog)
	var filteredPcrDetails = pfutil.IncludeModulesToEventLog(
		allPcrDetails, osModules)

	newMeta, err := pfutil.GetMetaSectionDetails(rhelpf.HostInfo, rhelpf.TagCertificate, "", cf.FlavorPartOs,
		hcConstants.VendorIntel)
	if err != nil {
		return nil, errors.Wrap(err, errorMessage+" Failure in Meta section details")
	}
	log.Debugf("flavor/types/linux_platform_flavor:getOsFlavor() New Meta Section: %v", *newMeta)

	newBios := pfutil.GetBiosSectionDetails(rhelpf.HostInfo)
	if newBios == nil {
		return nil, errors.Errorf("%s Failure in Bios section details", errorMessage)
	}
	log.Debugf("flavor/types/linux_platform_flavor:getOsFlavor() New Bios Section: %v", *newBios)

	// Assemble the OS Flavor
	osFlavor := cm.NewFlavor(newMeta, newBios, nil, filteredPcrDetails, nil, nil)

	log.Debugf("flavor/types/linux_platform_flavor:getOSFlavor()  New OS Flavor: %v", osFlavor)

	return []cm.Flavor{*osFlavor}, nil
}

// getHostUniqueFlavor Returns a json document having all the good known PCR values and corresponding event logs that
// can be used for evaluating the unique part of the PCR configurations of a host. These include PCRs/modules getting
// extended to PCRs that would vary from host to host.
func (rhelpf LinuxPlatformFlavor) getHostUniqueFlavor() ([]cm.Flavor, error) {
	log.Trace("flavor/types/linux_platform_flavor:getHostUniqueFlavor() Entering")
	defer log.Trace("flavor/types/linux_platform_flavor:getHostUniqueFlavor() Leaving")

	var errorMessage = "Error during creation of HOST_UNIQUE flavor"
	var err error
	var hostUniquePcrs = rhelpf.getPcrList(cf.FlavorPartHostUnique)
	var includeEventLog = rhelpf.eventLogRequired(cf.FlavorPartHostUnique)
	var allPcrDetails = pfutil.GetPcrDetails(
		rhelpf.HostManifest.PcrManifest, hostUniquePcrs, includeEventLog)
	var filteredPcrDetails = pfutil.IncludeModulesToEventLog(
		allPcrDetails, hostUniqueModules)

	newMeta, err := pfutil.GetMetaSectionDetails(rhelpf.HostInfo, rhelpf.TagCertificate, "", cf.FlavorPartHostUnique,
		hcConstants.VendorIntel)
	if err != nil {
		return nil, errors.Wrap(err, errorMessage+" Failure in Meta section details")
	}
	log.Debugf("flavor/types/linux_platform_flavor:getHostUniqueFlavor() New Meta Section: %v", *newMeta)

	newBios := pfutil.GetBiosSectionDetails(rhelpf.HostInfo)
	if newBios == nil {
		return nil, errors.Wrap(err, errorMessage+" Failure in Bios section details")
	}
	log.Debugf("flavor/types/linux_platform_flavor:getHostUniqueFlavor() New Bios Section: %v", *newBios)

	// Assemble the Host Unique Flavor
	hostUniqueFlavor := cm.NewFlavor(newMeta, newBios, nil, filteredPcrDetails, nil, nil)

	log.Debugf("flavor/types/esx_platform_flavor:getHostUniqueFlavor() New PlatformFlavor: %v", hostUniqueFlavor)

	return []cm.Flavor{*hostUniqueFlavor}, nil
}

// getAssetTagFlavor Retrieves the asset tag part of the flavor including the certificate and all the key-value pairs
// that are part of the certificate.
func (rhelpf LinuxPlatformFlavor) getAssetTagFlavor() ([]cm.Flavor, error) {
	log.Trace("flavor/types/linux_platform_flavor:getAssetTagFlavor() Entering")
	defer log.Trace("flavor/types/linux_platform_flavor:getAssetTagFlavor() Leaving")

	var errorMessage = "Error during creation of ASSET_TAG flavor"
	var err error

	if rhelpf.TagCertificate == nil {
		return nil, errors.Errorf("%s - %s", errorMessage, cf.FLAVOR_PART_CANNOT_BE_SUPPORTED().Message)
	}

	// create meta section details
	newMeta, err := pfutil.GetMetaSectionDetails(rhelpf.HostInfo, rhelpf.TagCertificate, "", cf.FlavorPartAssetTag,
		hcConstants.VendorIntel)
	if err != nil {
		return nil, errors.Wrap(err, errorMessage+" Failure in Meta section details")
	}
	log.Debugf("flavor/types/linux_platform_flavor:getAssetTagFlavor() New Meta Section: %v", *newMeta)

	// create bios section details
	newBios := pfutil.GetBiosSectionDetails(rhelpf.HostInfo)
	if newBios == nil {
		return nil, errors.Errorf("%s - Failure in Bios section details", errorMessage)
	}
	log.Debugf("flavor/types/linux_platform_flavor:getAssetTagFlavor() New Bios Section: %v", *newBios)

	// create external section details
	newExt, err := pfutil.GetExternalConfigurationDetails(rhelpf.TagCertificate)
	if err != nil {
		return nil, errors.Wrap(err, errorMessage+" Failure in External configuration section details")
	}
	log.Debugf("flavor/types/linux_platform_flavor:getAssetTagFlavor() New External Section: %v", *newExt)

	// Assemble the Asset Tag Flavor
	assetTagFlavor := cm.NewFlavor(newMeta, newBios, nil, nil, newExt, nil)

	log.Debugf("flavor/types/esx_platform_flavor:getPlatformFlavor() New Asset Tag Flavor: %v", assetTagFlavor)

	return []cm.Flavor{*assetTagFlavor}, nil
}

// getDefaultSoftwareFlavor Method to create a software flavor. This method would create a software flavor that would
// include all the measurements provided from host.
func (rhelpf LinuxPlatformFlavor) getDefaultSoftwareFlavor() ([]cm.Flavor, error) {
	log.Trace("flavor/types/linux_platform_flavor:getDefaultSoftwareFlavor() Entering")
	defer log.Trace("flavor/types/linux_platform_flavor:getDefaultSoftwareFlavor() Leaving")

	var softwareFlavors []cm.Flavor
	var errorMessage = cf.SOFTWARE_FLAVOR_CANNOT_BE_CREATED().Message

	if rhelpf.HostManifest != nil && rhelpf.HostManifest.MeasurementXmls != nil {
		measurementXmls, err := rhelpf.getDefaultMeasurement()
		if err != nil {
			return nil, errors.Wrapf(err, errorMessage)
		}

		for _, measurementXml := range measurementXmls {
			var softwareFlavor = NewSoftwareFlavor(measurementXml)
			swFlavor, err := softwareFlavor.GetSoftwareFlavor()
			if err != nil {
				return nil, err
			}
			softwareFlavors = append(softwareFlavors, *swFlavor)
		}
	}
	log.Debugf("flavor/types/esx_platform_flavor:getDefaultSoftwareFlavor() New Software Flavor: %v", softwareFlavors)
	return softwareFlavors, nil
}

// getDefaultMeasurement returns a default set of measurements for the Platform Flavor
func (rhelpf LinuxPlatformFlavor) getDefaultMeasurement() ([]string, error) {
	log.Trace("flavor/types/linux_platform_flavor:getDefaultMeasurement() Entering")
	defer log.Trace("flavor/types/linux_platform_flavor:getDefaultMeasurement() Leaving")

	var measurementXmlCollection []string
	var err error

	for _, measurementXML := range rhelpf.HostManifest.MeasurementXmls {
		var measurement taModel.Measurement
		err = xml.Unmarshal([]byte(measurementXML), &measurement)
		if err != nil {
			return nil, errors.Wrapf(err, "Error unmarshalling measurement XML: %s", err.Error())
		}
		if strings.Contains(measurement.Label, constants.DefaultSoftwareFlavorPrefix) ||
			strings.Contains(measurement.Label, constants.DefaultWorkloadFlavorPrefix) {
			measurementXmlCollection = append(measurementXmlCollection, measurementXML)
			log.Debugf("flavor/types/esx_platform_flavor:getDefaultMeasurement() Measurement XML: %s", measurementXML)
		}
	}
	return measurementXmlCollection, nil
}
