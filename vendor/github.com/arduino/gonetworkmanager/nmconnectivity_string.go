// Code generated by "stringer -type=NmConnectivity"; DO NOT EDIT

package gonetworkmanager

import "fmt"

const _NmConnectivity_name = "NmConnectivityUnknownNmConnectivityNoneNmConnectivityPortalNmConnectivityLimitedNmConnectivityFull"

var _NmConnectivity_index = [...]uint8{0, 21, 39, 59, 80, 98}

func (i NmConnectivity) String() string {
	if i >= NmConnectivity(len(_NmConnectivity_index)-1) {
		return fmt.Sprintf("NmConnectivity(%d)", i)
	}
	return _NmConnectivity_name[_NmConnectivity_index[i]:_NmConnectivity_index[i+1]]
}
