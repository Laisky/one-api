// editModalConfig holds the Berry channel editor's form validation so the EditModal component
// file only owns form state and rendering.
import * as Yup from 'yup';

/**
 * isClockHHMM reports whether the supplied value is a 24-hour HH:MM clock string.
 *
 * @param {unknown} value The candidate time-window boundary.
 * @returns {boolean} True when the value is a string such as "09:30" or "23:59".
 */
export const isClockHHMM = (value) => typeof value === 'string' && /^([01]\d|2[0-3]):[0-5]\d$/.test(value);

// MODEL_CONFIG_RECOGNIZED_KEYS lists the model_configs entry keys the backend understands: the JSON
// keys of model.ModelConfigLocal (model/channel_pricing_config.go). Every field is optional
// server-side, so an entry such as {"image":{"price_per_image_usd":0.2}}, or the {} the legacy
// image-price migration writes, is valid.
export const MODEL_CONFIG_RECOGNIZED_KEYS = new Set([
  'per_call',
  'per_page',
  'ratio',
  'completion_ratio',
  'cached_input_ratio',
  'cache_write_5m_ratio',
  'cache_write_1h_ratio',
  'tiers',
  'max_tokens',
  'context_length',
  'video',
  'audio',
  'image',
  'embedding',
  'time_windows'
]);

// validationSchema is the Formik/Yup schema EditModal validates the channel form with before submitting.
export const validationSchema = Yup.object().shape({
  is_edit: Yup.boolean(),
  name: Yup.string().required('名称 不能为空'),
  type: Yup.number().required('渠道 不能为空'),
  key: Yup.string().when(['is_edit', 'type'], {
    is: (is_edit, type) => !is_edit && type !== 33,
    then: Yup.string().required('密钥 不能为空')
  }),
  other: Yup.string(),
  models: Yup.array().min(1, '模型 不能为空'),
  groups: Yup.array().min(1, '用户组 不能为空'),
  base_url: Yup.string().when('type', {
    is: (value) => [3, 50].includes(value),
    then: Yup.string().required('渠道API地址 不能为空'), // base_url 是必需的
    otherwise: Yup.string() // 在其他情况下，base_url 可以是任意字符串
  }),
  model_mapping: Yup.string().test('is-json', '必须是有效的JSON字符串', function (value) {
    try {
      if (value === '' || value === null || value === undefined) {
        return true;
      }
      const parsedValue = JSON.parse(value);
      if (typeof parsedValue === 'object') {
        return true;
      }
    } catch (e) {
      return false;
    }
    return false;
  }),
  // model_ratio and completion_ratio validation removed - now handled through model_configs
  model_configs: Yup.string().nullable().test('is-valid-model-configs', '模型配置格式无效', function (value) {
    if (value === '' || value === null || value === undefined) {
      return true;
    }

    try {
      const configs = JSON.parse(value);

      if (typeof configs !== 'object' || configs === null || Array.isArray(configs)) {
        return this.createError({ message: '模型配置必须是JSON对象' });
      }

      for (const [modelName, config] of Object.entries(configs)) {
        if (!modelName || modelName.trim() === '') {
          return this.createError({ message: '模型名称不能为空' });
        }

        if (typeof config !== 'object' || config === null || Array.isArray(config)) {
          return this.createError({ message: `模型"${modelName}"的配置必须是对象` });
        }

        // Validate ratio
        if (config.ratio !== undefined) {
          if (typeof config.ratio !== 'number' || config.ratio < 0) {
            return this.createError({ message: `模型"${modelName}"的ratio无效：必须是非负数` });
          }
        }

        // Validate completion_ratio
        if (config.completion_ratio !== undefined) {
          if (typeof config.completion_ratio !== 'number' || config.completion_ratio < 0) {
            return this.createError({ message: `模型"${modelName}"的completion_ratio无效：必须是非负数` });
          }
        }

        // Validate max_tokens
        if (config.max_tokens !== undefined) {
          if (!Number.isInteger(config.max_tokens) || config.max_tokens < 0) {
            return this.createError({ message: `模型"${modelName}"的max_tokens无效：必须是非负整数` });
          }
        }

        if (config.time_windows !== undefined) {
          if (!Array.isArray(config.time_windows)) {
            return this.createError({ message: `Model "${modelName}" time_windows must be an array` });
          }
          for (const [index, window] of config.time_windows.entries()) {
            if (typeof window !== 'object' || window === null || Array.isArray(window)) {
              return this.createError({ message: `Model "${modelName}" time window ${index + 1} must be an object` });
            }
            if (!Array.isArray(window.ranges) || window.ranges.length === 0) {
              return this.createError({ message: `Model "${modelName}" time window ${index + 1} ranges must be a non-empty array` });
            }
            for (const range of window.ranges) {
              if (typeof range !== 'object' || range === null || !isClockHHMM(range.start) || !isClockHHMM(range.end)) {
                return this.createError({ message: `Model "${modelName}" time window ${index + 1} ranges must use HH:MM strings` });
              }
            }
            if (typeof window.overlay !== 'object' || window.overlay === null || Array.isArray(window.overlay)) {
              return this.createError({ message: `Model "${modelName}" time window ${index + 1} overlay must be an object` });
            }
          }
        }

        // Accept an empty entry or one with at least one recognized key; reject only a non-empty
        // entry whose keys are all unrecognized, which is almost always a typo the backend ignores.
        const entryKeys = Object.keys(config);
        if (entryKeys.length > 0 && !entryKeys.some((key) => MODEL_CONFIG_RECOGNIZED_KEYS.has(key))) {
          return this.createError({
            message: `Model "${modelName}" has no recognized configuration keys (unknown: ${entryKeys.join(', ')})`
          });
        }
      }

      return true;
    } catch (error) {
      return this.createError({ message: `JSON格式无效：${error.message}` });
    }
  }),
  inference_profile_arn_map: Yup.string().nullable().test('is-json', '必须是有效的JSON字符串', function (value) {
    try {
      if (value === '' || value === null || value === undefined) {
        return true;
      }
      const parsedValue = JSON.parse(value);
      if (typeof parsedValue === 'object') {
        return true;
      }
    } catch (e) {
      return false;
    }
    return false;
  })
});
