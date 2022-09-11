package dmr

const remoteControlServiceDescription = `<?xml version="1.0" encoding="utf-8"?>
	<scpd xmlns="urn:schemas-upnp-org:service-1-0">
		<specVersion>
			<major>1</major>
			<minor>0</minor>
		</specVersion>
	<actionList/>
	<serviceStateTable>
		<stateVariable sendEvents="no">
			<name>X_RController</name>
			<dataType>string</dataType>
		</stateVariable>
	</serviceStateTable>
</scpd>`
