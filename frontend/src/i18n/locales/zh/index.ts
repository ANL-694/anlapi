import landing from './landing'
import common from './common'
import dashboard from './dashboard'
import channelMonitorV2 from './channelMonitorV2'
import batchImage from './batchImage'
import admin from './admin'
import misc from './misc'
import httpStatusCodes from './httpStatusCodes'
import store from './store'
import userAccounts from './userAccounts'

export default {
  ...landing,
  ...common,
  ...dashboard,
  ...channelMonitorV2,
  ...batchImage,
  admin,
  ...misc,
  ...httpStatusCodes,
  ...store,
  ...userAccounts,
}
