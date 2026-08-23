export const LOCALE_COOKIE = 'rakda_locale';

export const LOCALES = ['id', 'en'] as const;
export type Locale = (typeof LOCALES)[number];

export const defaultLocale: Locale = 'id';

export const localeLabels: Record<Locale, string> = {
	id: 'Indonesia',
	en: 'English'
};

export function isLocale(value: unknown): value is Locale {
	return typeof value === 'string' && (LOCALES as readonly string[]).includes(value);
}
