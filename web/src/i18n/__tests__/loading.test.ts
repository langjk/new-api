import { createInstance } from 'i18next'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

describe('interface translation loading', () => {
  beforeEach(() => {
    vi.resetModules()
    // Isolate the real engine from the global instance initialized by test-setup.
    vi.doMock('i18next', () => ({ default: createInstance() }))
    localStorage.clear()
    localStorage.setItem('i18nextLng', 'zh-CN')
  })

  afterEach(() => {
    localStorage.clear()
    vi.doUnmock('../locales/fr.json')
    vi.doUnmock('i18next')
  })

  it('starts in Chinese without loading the five unused language dictionaries', async () => {
    const { default: i18n } = await import('../config')
    expect(i18n.language).toBe('zhCN')
    expect(i18n.t('Cancel')).toBe('取消')
    expect(Object.keys(i18n.store.data).sort()).toEqual(['en', 'zhCN'])
  })

  it('loads a selected language and reuses it on later switches', async () => {
    const { default: i18n } = await import('../config')
    expect(i18n.hasResourceBundle('fr', 'translation')).toBe(false)
    await i18n.changeLanguage('fr')
    expect(i18n.t('Cancel')).toBe('Annuler')
    const french = i18n.getResourceBundle('fr', 'translation')
    await i18n.changeLanguage('zhCN')
    expect(i18n.t('Cancel')).toBe('取消')
    await i18n.changeLanguage('fr')
    expect(i18n.getResourceBundle('fr', 'translation')).toBe(french)
    expect(i18n.hasResourceBundle('ru', 'translation')).toBe(false)
  })

  it('keeps the English fallback usable when a language download fails', async () => {
    vi.doMock('../locales/fr.json', () => {
      throw new Error('Language chunk unavailable')
    })
    const { default: i18n } = await import('../config')
    await i18n.changeLanguage('fr')
    expect(i18n.t('Cancel')).toBe('Cancel')
    await i18n.changeLanguage('zhCN')
    expect(i18n.t('Cancel')).toBe('取消')
  })

  it.each(['fr', 'ja', 'ru', 'vi', 'zhTW'])(
    'restores a saved %s preference on a fresh visit',
    async (language) => {
      localStorage.setItem('i18nextLng', language)
      const { default: i18n, i18nReady } = await import('../config')
      await i18nReady
      expect(i18n.resolvedLanguage).toBe(language)
      expect(i18n.t('Cancel')).not.toBe('Cancel')
      expect(Object.keys(i18n.store.data).sort()).toEqual(
        ['en', 'zhCN', language].sort()
      )
    }
  )
})
