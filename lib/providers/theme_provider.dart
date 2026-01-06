import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:spotiflac_android/models/theme_settings.dart';

/// Provider for theme settings state management
final themeProvider = NotifierProvider<ThemeNotifier, ThemeSettings>(() {
  return ThemeNotifier();
});

/// Notifier for managing theme settings with persistence
class ThemeNotifier extends Notifier<ThemeSettings> {
  @override
  ThemeSettings build() {
    // Load settings asynchronously on first access
    _loadFromStorage();
    return const ThemeSettings();
  }

  /// Load theme settings from SharedPreferences
  Future<void> _loadFromStorage() async {
    try {
      final prefs = await SharedPreferences.getInstance();
      final modeString = prefs.getString(kThemeModeKey);
      final useDynamic = prefs.getBool(kUseDynamicColorKey);
      final seedColor = prefs.getInt(kSeedColorKey);
      final useAmoled = prefs.getBool(kUseAmoledKey);

      state = ThemeSettings(
        themeMode: _themeModeFromString(modeString),
        useDynamicColor: useDynamic ?? true,
        seedColorValue: seedColor ?? kDefaultSeedColor,
        useAmoled: useAmoled ?? false,
      );
    } catch (e) {
      debugPrint('Error loading theme settings: $e');
      // Keep default state on error
    }
  }

  /// Save current settings to SharedPreferences
  Future<void> _saveToStorage() async {
    try {
      final prefs = await SharedPreferences.getInstance();
      await prefs.setString(kThemeModeKey, state.themeMode.name);
      await prefs.setBool(kUseDynamicColorKey, state.useDynamicColor);
      await prefs.setInt(kSeedColorKey, state.seedColorValue);
      await prefs.setBool(kUseAmoledKey, state.useAmoled);
    } catch (e) {
      debugPrint('Error saving theme settings: $e');
    }
  }

  /// Set theme mode (light, dark, or system)
  Future<void> setThemeMode(ThemeMode mode) async {
    state = state.copyWith(themeMode: mode);
    await _saveToStorage();
  }

  /// Enable or disable dynamic color from wallpaper
  Future<void> setUseDynamicColor(bool value) async {
    state = state.copyWith(useDynamicColor: value);
    await _saveToStorage();
  }

  /// Set custom seed color (used when dynamic color is disabled)
  Future<void> setSeedColor(Color color) async {
    state = state.copyWith(seedColorValue: color.toARGB32());
    await _saveToStorage();
  }

  /// Set seed color from int value
  Future<void> setSeedColorValue(int colorValue) async {
    state = state.copyWith(seedColorValue: colorValue);
    await _saveToStorage();
  }

  /// Enable or disable AMOLED mode (pure black background)
  Future<void> setUseAmoled(bool value) async {
    state = state.copyWith(useAmoled: value);
    await _saveToStorage();
  }

  /// Helper to convert string to ThemeMode
  ThemeMode _themeModeFromString(String? value) {
    if (value == null) return ThemeMode.system;
    return ThemeMode.values.firstWhere(
      (e) => e.name == value,
      orElse: () => ThemeMode.system,
    );
  }
}
