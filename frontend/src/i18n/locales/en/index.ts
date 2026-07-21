import landing from './landing'
import common from './common'
import dashboard from './dashboard'
import batchImage from './batchImage'
import asyncImageTasks from './asyncImageTasks'
import imageWorkflow from './imageWorkflow'
import admin from './admin'
import misc from './misc'

export default {
  ...landing,
  ...common,
  ...dashboard,
  ...batchImage,
  ...asyncImageTasks,
  ...imageWorkflow,
  admin,
  ...misc,
}
