import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:spotiflac_android/providers/settings_provider.dart';
import 'package:spotiflac_android/providers/theme_provider.dart';
import 'package:spotiflac_android/widgets/settings_group.dart';

class AppearanceSettingsPage extends ConsumerWidget {
  const AppearanceSettingsPage({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final themeSettings = ref.watch(themeProvider);
    final settings = ref.watch(settingsProvider);
    final colorScheme = Theme.of(context).colorScheme;
    final topPadding = MediaQuery.of(context).padding.top;

    return PopScope(
      canPop: true,
      child: Scaffold(
        body: CustomScrollView(
          slivers: [
            // Collapsing App Bar with back button
            SliverAppBar(
              expandedHeight: 120 + topPadding,
              collapsedHeight: kToolbarHeight,
              floating: false,
              pinned: true,
              backgroundColor: colorScheme.surface,
              surfaceTintColor: Colors.transparent,
              leading: IconButton(icon: const Icon(Icons.arrow_back), onPressed: () => Navigator.pop(context)),
              flexibleSpace: _AppBarTitle(title: 'Appearance', topPadding: topPadding),
            ),

            // Theme section
            const SliverToBoxAdapter(child: SettingsSectionHeader(title: 'Theme')),
            SliverToBoxAdapter(
              child: SettingsGroup(
                children: [
                  _ThemeModeSelector(
                    currentMode: themeSettings.themeMode,
                    onChanged: (mode) => ref.read(themeProvider.notifier).setThemeMode(mode),
                  ),
                  SettingsSwitchItem(
                    icon: Icons.brightness_2,
                    title: 'AMOLED Dark',
                    subtitle: 'Pure black background for OLED screens',
                    value: themeSettings.useAmoled,
                    onChanged: (value) => ref.read(themeProvider.notifier).setUseAmoled(value),
                    showDivider: false,
                  ),
                ],
              ),
            ),

            // Color section
            const SliverToBoxAdapter(child: SettingsSectionHeader(title: 'Color')),
            SliverToBoxAdapter(
              child: SettingsGroup(
                children: [
                  SettingsSwitchItem(
                    icon: Icons.auto_awesome,
                    title: 'Dynamic Color',
                    subtitle: 'Use colors from your wallpaper',
                    value: themeSettings.useDynamicColor,
                    onChanged: (value) => ref.read(themeProvider.notifier).setUseDynamicColor(value),
                    showDivider: !themeSettings.useDynamicColor,
                  ),
                  if (!themeSettings.useDynamicColor)
                    _ColorPicker(
                      currentColor: themeSettings.seedColorValue,
                      onColorSelected: (color) => ref.read(themeProvider.notifier).setSeedColor(color),
                    ),
                ],
              ),
            ),

            // Layout section
            const SliverToBoxAdapter(child: SettingsSectionHeader(title: 'Layout')),
            SliverToBoxAdapter(
              child: SettingsGroup(
                children: [
                  _HistoryViewSelector(
                    currentMode: settings.historyViewMode,
                    onChanged: (mode) => ref.read(settingsProvider.notifier).setHistoryViewMode(mode),
                  ),
                ],
              ),
            ),

            // Fill remaining for scroll
            const SliverFillRemaining(hasScrollBody: false, child: SizedBox()),
          ],
        ),
      ),
    );
  }
}

/// Optimized app bar title with animation
class _AppBarTitle extends StatelessWidget {
  final String title;
  final double topPadding;
  
  const _AppBarTitle({required this.title, required this.topPadding});

  @override
  Widget build(BuildContext context) {
    final colorScheme = Theme.of(context).colorScheme;
    return LayoutBuilder(
      builder: (context, constraints) {
        final maxHeight = 120 + topPadding;
        final minHeight = kToolbarHeight + topPadding;
        final expandRatio = ((constraints.maxHeight - minHeight) / (maxHeight - minHeight)).clamp(0.0, 1.0);
        final leftPadding = 56 - (32 * expandRatio); // 56 -> 24
        return FlexibleSpaceBar(
          expandedTitleScale: 1.0,
          titlePadding: EdgeInsets.only(left: leftPadding, bottom: 16),
          title: Text(
            title,
            style: TextStyle(
              fontSize: 20 + (8 * expandRatio), // 20 -> 28
              fontWeight: FontWeight.bold,
              color: colorScheme.onSurface,
            ),
          ),
        );
      },
    );
  }
}

class _ThemeModeSelector extends StatelessWidget {
  final ThemeMode currentMode;
  final ValueChanged<ThemeMode> onChanged;
  const _ThemeModeSelector({required this.currentMode, required this.onChanged});

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.all(12),
      child: Row(children: [
        _ThemeModeChip(icon: Icons.brightness_auto, label: 'System', isSelected: currentMode == ThemeMode.system, onTap: () => onChanged(ThemeMode.system)),
        const SizedBox(width: 8),
        _ThemeModeChip(icon: Icons.light_mode, label: 'Light', isSelected: currentMode == ThemeMode.light, onTap: () => onChanged(ThemeMode.light)),
        const SizedBox(width: 8),
        _ThemeModeChip(icon: Icons.dark_mode, label: 'Dark', isSelected: currentMode == ThemeMode.dark, onTap: () => onChanged(ThemeMode.dark)),
      ]),
    );
  }
}

class _ThemeModeChip extends StatelessWidget {
  final IconData icon;
  final String label;
  final bool isSelected;
  final VoidCallback onTap;
  const _ThemeModeChip({required this.icon, required this.label, required this.isSelected, required this.onTap});

  @override
  Widget build(BuildContext context) {
    final colorScheme = Theme.of(context).colorScheme;
    final isDark = Theme.of(context).brightness == Brightness.dark;
    
    // Unselected chips need contrast with card background
    // Card uses: dark = white 8% overlay, light = surfaceContainerHighest
    // So chips use: dark = white 5% overlay (darker), light = black 5% overlay (darker than card)
    final unselectedColor = isDark 
        ? Color.alphaBlend(Colors.white.withValues(alpha: 0.05), colorScheme.surface)
        : Color.alphaBlend(Colors.black.withValues(alpha: 0.05), colorScheme.surfaceContainerHighest);
    
    return Expanded(
      child: Container(
        decoration: BoxDecoration(
          color: isSelected ? colorScheme.primaryContainer : unselectedColor,
          borderRadius: BorderRadius.circular(12),
          border: !isDark && !isSelected 
              ? Border.all(color: colorScheme.outlineVariant.withValues(alpha: 0.5), width: 1)
              : null,
        ),
        child: Material(
          color: Colors.transparent,
          borderRadius: BorderRadius.circular(12),
          child: InkWell(
            onTap: onTap,
            borderRadius: BorderRadius.circular(12),
            child: Padding(
              padding: const EdgeInsets.symmetric(vertical: 14),
              child: Column(children: [
                Icon(icon, color: isSelected ? colorScheme.onPrimaryContainer : colorScheme.onSurfaceVariant),
                const SizedBox(height: 6),
                Text(label, style: TextStyle(fontSize: 12,
                  fontWeight: isSelected ? FontWeight.w600 : FontWeight.normal,
                  color: isSelected ? colorScheme.onPrimaryContainer : colorScheme.onSurfaceVariant)),
              ]),
            ),
          ),
        ),
      ),
    );
  }
}

class _ColorPicker extends StatelessWidget {
  final int currentColor;
  final ValueChanged<Color> onColorSelected;
  const _ColorPicker({required this.currentColor, required this.onColorSelected});

  static const _colors = [
    Color(0xFF1DB954), Color(0xFF6750A4), Color(0xFF0061A4), Color(0xFF006E1C),
    Color(0xFFBA1A1A), Color(0xFF984061), Color(0xFF7D5260), Color(0xFF006874), Color(0xFFFF6F00),
  ];

  @override
  Widget build(BuildContext context) {
    final colorScheme = Theme.of(context).colorScheme;
    return Padding(
      padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Text('Accent Color', style: Theme.of(context).textTheme.bodyMedium?.copyWith(color: colorScheme.onSurfaceVariant)),
        const SizedBox(height: 12),
        Wrap(spacing: 12, runSpacing: 12, children: _colors.map((color) {
          final isSelected = color.toARGB32() == currentColor;
          return GestureDetector(
            onTap: () => onColorSelected(color),
            child: AnimatedContainer(
              duration: const Duration(milliseconds: 200),
              width: 44, height: 44,
              decoration: BoxDecoration(
                color: color, shape: BoxShape.circle,
                border: isSelected ? Border.all(color: colorScheme.onSurface, width: 3) : null,
                boxShadow: isSelected ? [BoxShadow(color: color.withValues(alpha: 0.4), blurRadius: 8, spreadRadius: 2)] : null,
              ),
              child: isSelected ? const Icon(Icons.check, color: Colors.white, size: 20) : null,
            ),
          );
        }).toList()),
      ]),
    );
  }
}

class _HistoryViewSelector extends StatelessWidget {
  final String currentMode;
  final ValueChanged<String> onChanged;
  const _HistoryViewSelector({required this.currentMode, required this.onChanged});

  @override
  Widget build(BuildContext context) {
    final colorScheme = Theme.of(context).colorScheme;
    return Padding(
      padding: const EdgeInsets.all(12),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Padding(
            padding: const EdgeInsets.only(left: 8, bottom: 8),
            child: Text('History View', style: Theme.of(context).textTheme.bodyMedium?.copyWith(color: colorScheme.onSurfaceVariant)),
          ),
          Row(children: [
            _ViewModeChip(icon: Icons.view_list, label: 'List', isSelected: currentMode == 'list', onTap: () => onChanged('list')),
            const SizedBox(width: 8),
            _ViewModeChip(icon: Icons.grid_view, label: 'Grid', isSelected: currentMode == 'grid', onTap: () => onChanged('grid')),
          ]),
        ],
      ),
    );
  }
}

class _ViewModeChip extends StatelessWidget {
  final IconData icon;
  final String label;
  final bool isSelected;
  final VoidCallback onTap;
  const _ViewModeChip({required this.icon, required this.label, required this.isSelected, required this.onTap});

  @override
  Widget build(BuildContext context) {
    final colorScheme = Theme.of(context).colorScheme;
    final isDark = Theme.of(context).brightness == Brightness.dark;
    
    // Unselected chips need contrast with card background
    final unselectedColor = isDark 
        ? Color.alphaBlend(Colors.white.withValues(alpha: 0.05), colorScheme.surface)
        : Color.alphaBlend(Colors.black.withValues(alpha: 0.05), colorScheme.surfaceContainerHighest);
    
    return Expanded(
      child: Container(
        decoration: BoxDecoration(
          color: isSelected ? colorScheme.primaryContainer : unselectedColor,
          borderRadius: BorderRadius.circular(12),
          border: !isDark && !isSelected 
              ? Border.all(color: colorScheme.outlineVariant.withValues(alpha: 0.5), width: 1)
              : null,
        ),
        child: Material(
          color: Colors.transparent,
          borderRadius: BorderRadius.circular(12),
          child: InkWell(
            onTap: onTap,
            borderRadius: BorderRadius.circular(12),
            child: Padding(
              padding: const EdgeInsets.symmetric(vertical: 14),
              child: Column(children: [
                Icon(icon, color: isSelected ? colorScheme.onPrimaryContainer : colorScheme.onSurfaceVariant),
                const SizedBox(height: 6),
                Text(label, style: TextStyle(fontSize: 12,
                  fontWeight: isSelected ? FontWeight.w600 : FontWeight.normal,
                  color: isSelected ? colorScheme.onPrimaryContainer : colorScheme.onSurfaceVariant)),
              ]),
            ),
          ),
        ),
      ),
    );
  }
}
