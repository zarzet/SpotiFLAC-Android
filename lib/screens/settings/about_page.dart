import 'package:flutter/material.dart';
import 'package:url_launcher/url_launcher.dart';
import 'package:cached_network_image/cached_network_image.dart';
import 'package:spotiflac_android/constants/app_info.dart';
import 'package:spotiflac_android/widgets/settings_group.dart';

class AboutPage extends StatelessWidget {
  const AboutPage({super.key});

  @override
  Widget build(BuildContext context) {
    final colorScheme = Theme.of(context).colorScheme;
    final topPadding = MediaQuery.of(context).padding.top;

    return PopScope(
      canPop: true, // Always allow back gesture
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
            leading: IconButton(
              icon: const Icon(Icons.arrow_back),
              onPressed: () => Navigator.pop(context),
            ),
            flexibleSpace: LayoutBuilder(
              builder: (context, constraints) {
                final maxHeight = 120 + topPadding;
                final minHeight = kToolbarHeight + topPadding;
                final expandRatio = ((constraints.maxHeight - minHeight) / (maxHeight - minHeight)).clamp(0.0, 1.0);
                // When collapsed (expandRatio=0): left=56 to avoid back button
                // When expanded (expandRatio=1): left=24 for normal padding
                final leftPadding = 56 - (32 * expandRatio); // 56 -> 24
                return FlexibleSpaceBar(
                  expandedTitleScale: 1.0,
                  titlePadding: EdgeInsets.only(left: leftPadding, bottom: 16),
                  title: Text(
                    'About',
                    style: TextStyle(
                      fontSize: 20 + (8 * expandRatio), // 20 -> 28
                      fontWeight: FontWeight.bold,
                      color: colorScheme.onSurface,
                    ),
                  ),
                );
              },
            ),
          ),

          // App header card with logo and description
          SliverToBoxAdapter(
            child: Padding(
              padding: const EdgeInsets.fromLTRB(16, 16, 16, 8),
              child: _AppHeaderCard(),
            ),
          ),

          // Contributors section
          const SliverToBoxAdapter(
            child: SettingsSectionHeader(title: 'Contributors'),
          ),
          SliverToBoxAdapter(
            child: SettingsGroup(
              children: [
                _ContributorItem(
                  name: AppInfo.mobileAuthor,
                  description: 'Mobile version developer',
                  githubUsername: AppInfo.mobileAuthor,
                  showDivider: true,
                ),
                _ContributorItem(
                  name: AppInfo.originalAuthor,
                  description: 'Creator of the original SpotiFLAC',
                  githubUsername: AppInfo.originalAuthor,
                  showDivider: true,
                ),
                _ContributorItem(
                  name: 'Amonoman',
                  description: 'The talented artist who created our beautiful app logo!',
                  githubUsername: 'Amonoman',
                  showDivider: false,
                ),
              ],
            ),
          ),

          // Special Thanks section
          const SliverToBoxAdapter(
            child: SettingsSectionHeader(title: 'Special Thanks'),
          ),
          SliverToBoxAdapter(
            child: SettingsGroup(
              children: [
                _ContributorItem(
                  name: 'uimaxbai',
                  description: 'The creator of QQDL & HiFi API. Without this API, Tidal downloads wouldn\'t exist!',
                  githubUsername: 'uimaxbai',
                  showDivider: true,
                ),
                _ContributorItem(
                  name: 'sachinsenal0x64',
                  description: 'The original HiFi project creator. The foundation of Tidal integration!',
                  githubUsername: 'sachinsenal0x64',
                  showDivider: true,
                ),
                _AboutSettingsItem(
                  icon: Icons.cloud_outlined,
                  title: 'DoubleDouble',
                  subtitle: 'Amazing API for Amazon Music downloads. Thank you for making it free!',
                  onTap: () => _launchUrl('https://doubledouble.top'),
                  showDivider: true,
                ),
                _AboutSettingsItem(
                  icon: Icons.music_note_outlined,
                  title: 'DAB Music',
                  subtitle: 'The best Qobuz streaming API. Hi-Res downloads wouldn\'t be possible without this!',
                  onTap: () => _launchUrl('https://dabmusic.xyz'),
                  showDivider: false,
                ),
              ],
            ),
          ),

          // Links section
          const SliverToBoxAdapter(
            child: SettingsSectionHeader(title: 'Links'),
          ),
          SliverToBoxAdapter(
            child: SettingsGroup(
              children: [
                SettingsItem(
                  icon: Icons.phone_android,
                  title: 'Mobile source code',
                  subtitle: 'github.com/${AppInfo.githubRepo}',
                  onTap: () => _launchUrl(AppInfo.githubUrl),
                  showDivider: true,
                ),
                SettingsItem(
                  icon: Icons.computer,
                  title: 'PC source code',
                  subtitle: 'github.com/${AppInfo.originalAuthor}/SpotiFLAC',
                  onTap: () => _launchUrl(AppInfo.originalGithubUrl),
                  showDivider: true,
                ),
                SettingsItem(
                  icon: Icons.bug_report_outlined,
                  title: 'Report an issue',
                  subtitle: 'Report any problems you encounter',
                  onTap: () => _launchUrl('${AppInfo.githubUrl}/issues/new'),
                  showDivider: true,
                ),
                SettingsItem(
                  icon: Icons.lightbulb_outline,
                  title: 'Feature request',
                  subtitle: 'Suggest new features for the app',
                  onTap: () => _launchUrl('${AppInfo.githubUrl}/issues/new'),
                  showDivider: false,
                ),
              ],
            ),
          ),

          // Support section
          const SliverToBoxAdapter(
            child: SettingsSectionHeader(title: 'Support'),
          ),
          SliverToBoxAdapter(
            child: SettingsGroup(
              children: [
                SettingsItem(
                  icon: Icons.coffee_outlined,
                  title: 'Buy me a coffee',
                  subtitle: 'Support development on Ko-fi',
                  onTap: () => _launchUrl(AppInfo.kofiUrl),
                  showDivider: false,
                ),
              ],
            ),
          ),

          // App info section
          const SliverToBoxAdapter(
            child: SettingsSectionHeader(title: 'App'),
          ),
          SliverToBoxAdapter(
            child: SettingsGroup(
              children: [
                SettingsItem(
                  icon: Icons.info_outline,
                  title: 'Version',
                  subtitle: 'v${AppInfo.version} (build ${AppInfo.buildNumber})',
                  showDivider: false,
                ),
              ],
            ),
          ),

          // Copyright
          SliverToBoxAdapter(
            child: Padding(
              padding: const EdgeInsets.all(24),
              child: Center(
                child: Text(
                  AppInfo.copyright,
                  style: Theme.of(context).textTheme.bodySmall?.copyWith(
                    color: colorScheme.onSurfaceVariant,
                  ),
                ),
              ),
            ),
          ),

          // Bottom padding
          const SliverToBoxAdapter(child: SizedBox(height: 16)),
        ],
      ),
    ),
    );
  }

  static Future<void> _launchUrl(String url) async {
    final uri = Uri.parse(url);
    // Use inAppBrowserView for reliable URL opening with app chooser
    await launchUrl(uri, mode: LaunchMode.inAppBrowserView);
  }
}

class _AppHeaderCard extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    final colorScheme = Theme.of(context).colorScheme;
    final isDark = Theme.of(context).brightness == Brightness.dark;
    
    final cardColor = isDark 
        ? Color.alphaBlend(Colors.white.withValues(alpha: 0.08), colorScheme.surface)
        : colorScheme.surfaceContainerHighest;

    return Container(
      decoration: BoxDecoration(
        color: cardColor,
        borderRadius: BorderRadius.circular(20),
      ),
      padding: const EdgeInsets.all(24),
      child: Column(
        children: [
          // App logo
          // App logo
          Container(
            width: 88,
            height: 88,
            decoration: BoxDecoration(
              color: colorScheme.primary,
              shape: BoxShape.circle,
            ),
            child: Image.asset(
              'assets/images/logo-transparant.png',
              color: colorScheme.onPrimary, // Tint with onPrimary color
              fit: BoxFit.contain,
              errorBuilder: (_, _, _) => ClipRRect(
                borderRadius: BorderRadius.circular(24),
                child: Image.asset(
                  'assets/images/logo.png',
                  width: 88,
                  height: 88,
                  fit: BoxFit.cover,
                ),
              ),
            ),
          ),
          const SizedBox(height: 16),
          // App name
          Text(
            AppInfo.appName,
            style: Theme.of(context).textTheme.headlineSmall?.copyWith(
              fontWeight: FontWeight.bold,
            ),
          ),
          const SizedBox(height: 4),
          // Version badge
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 4),
            decoration: BoxDecoration(
              color: colorScheme.secondaryContainer,
              borderRadius: BorderRadius.circular(12),
            ),
            child: Text(
              'v${AppInfo.version}',
              style: Theme.of(context).textTheme.labelMedium?.copyWith(
                color: colorScheme.onSecondaryContainer,
                fontWeight: FontWeight.w600,
              ),
            ),
          ),
          const SizedBox(height: 16),
          // Description
          Text(
            'Download Spotify tracks in lossless quality from Tidal, Qobuz, and Amazon Music.',
            textAlign: TextAlign.center,
            style: Theme.of(context).textTheme.bodyMedium?.copyWith(
              color: colorScheme.onSurfaceVariant,
            ),
          ),
        ],
      ),
    );
  }
}

class _ContributorItem extends StatelessWidget {
  final String name;
  final String description;
  final String githubUsername;
  final bool showDivider;

  const _ContributorItem({
    required this.name,
    required this.description,
    required this.githubUsername,
    this.showDivider = false,
  });

  @override
  Widget build(BuildContext context) {
    final colorScheme = Theme.of(context).colorScheme;
    
    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        InkWell(
          onTap: () => _launchGitHub(githubUsername),
          splashColor: colorScheme.primary.withValues(alpha: 0.12),
          highlightColor: colorScheme.primary.withValues(alpha: 0.08),
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 12),
            child: Row(
              children: [
                // GitHub Avatar
                ClipRRect(
                  borderRadius: BorderRadius.circular(12),
                  child: CachedNetworkImage(
                    imageUrl: 'https://github.com/$githubUsername.png',
                    width: 40,
                    height: 40,
                    fit: BoxFit.cover,
                    placeholder: (context, url) => Container(
                      width: 40,
                      height: 40,
                      color: colorScheme.surfaceContainerHighest,
                      child: Icon(
                        Icons.person,
                        color: colorScheme.onSurfaceVariant,
                        size: 20,
                      ),
                    ),
                    errorWidget: (context, url, error) => Container(
                      width: 40,
                      height: 40,
                      color: colorScheme.surfaceContainerHighest,
                      child: Icon(
                        Icons.person,
                        color: colorScheme.onSurfaceVariant,
                        size: 20,
                      ),
                    ),
                  ),
                ),
                const SizedBox(width: 16),
                // Name and description
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        name,
                        style: Theme.of(context).textTheme.bodyLarge,
                      ),
                      const SizedBox(height: 2),
                      Text(
                        description,
                        style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                          color: colorScheme.onSurfaceVariant,
                        ),
                      ),
                    ],
                  ),
                ),
                // GitHub icon
                Icon(Icons.chevron_right, color: colorScheme.onSurfaceVariant),
              ],
            ),
          ),
        ),
        if (showDivider)
          Divider(
            height: 1,
            thickness: 1,
            indent: 76,
            endIndent: 20,
            color: colorScheme.outlineVariant.withValues(alpha: 0.3),
          ),
      ],
    );
  }

  Future<void> _launchGitHub(String username) async {
    final uri = Uri.parse('https://github.com/$username');
    await launchUrl(uri, mode: LaunchMode.inAppBrowserView);
  }
}

/// Settings item with 40x40 icon area to align with contributor avatars
class _AboutSettingsItem extends StatelessWidget {
  final IconData icon;
  final String title;
  final String? subtitle;
  final VoidCallback? onTap;
  final bool showDivider;

  const _AboutSettingsItem({
    required this.icon,
    required this.title,
    this.subtitle,
    this.onTap,
    this.showDivider = true,
  });

  @override
  Widget build(BuildContext context) {
    final colorScheme = Theme.of(context).colorScheme;
    
    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        InkWell(
          onTap: onTap,
          splashColor: colorScheme.primary.withValues(alpha: 0.12),
          highlightColor: colorScheme.primary.withValues(alpha: 0.08),
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 20, vertical: 12),
            child: Row(
              children: [
                // Icon with 40x40 size to match avatar
                SizedBox(
                  width: 40,
                  height: 40,
                  child: Icon(icon, color: colorScheme.onSurfaceVariant, size: 24),
                ),
                const SizedBox(width: 16),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        title,
                        style: Theme.of(context).textTheme.bodyLarge,
                      ),
                      if (subtitle != null) ...[
                        const SizedBox(height: 2),
                        Text(
                          subtitle!,
                          style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                            color: colorScheme.onSurfaceVariant,
                          ),
                        ),
                      ],
                    ],
                  ),
                ),
                if (onTap != null)
                  Icon(Icons.chevron_right, color: colorScheme.onSurfaceVariant),
              ],
            ),
          ),
        ),
        if (showDivider)
          Divider(
            height: 1,
            thickness: 1,
            indent: 76, // 20 + 40 + 16 = 76 (same as contributor item)
            endIndent: 20,
            color: colorScheme.outlineVariant.withValues(alpha: 0.3),
          ),
      ],
    );
  }
}
