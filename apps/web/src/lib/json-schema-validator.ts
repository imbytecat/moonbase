import { customizeValidator } from '@rjsf/validator-ajv8'
import Ajv2020 from 'ajv/dist/2020'

export const jsonSchemaValidator = customizeValidator({ AjvClass: Ajv2020 })
