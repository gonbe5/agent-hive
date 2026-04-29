// i18n TypeScript 类型增强
import 'react-i18next';
import type enTranslation from './locales/en.json';

declare module 'react-i18next' {
  interface CustomTypeOptions {
    defaultNS: 'translation';
    resources: {
      translation: typeof enTranslation;
    };
  }
}
