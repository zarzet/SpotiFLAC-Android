import 'dart:io';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:cached_network_image/cached_network_image.dart';
import 'package:palette_generator/palette_generator.dart';
import 'package:open_filex/open_filex.dart';
import 'package:spotiflac_android/l10n/l10n.dart';
import 'package:spotiflac_android/utils/mime_utils.dart';
import 'package:spotiflac_android/providers/download_queue_provider.dart';
import 'package:spotiflac_android/screens/track_metadata_screen.dart';

/// Screen to display downloaded tracks from a specific album
class DownloadedAlbumScreen extends ConsumerStatefulWidget {
  final String albumName;
  final String artistName;
  final String? coverUrl;

  const DownloadedAlbumScreen({
    super.key,
    required this.albumName,
    required this.artistName,
    this.coverUrl,
  });

  @override
  ConsumerState<DownloadedAlbumScreen> createState() => _DownloadedAlbumScreenState();
}

class _DownloadedAlbumScreenState extends ConsumerState<DownloadedAlbumScreen> {
  bool _isSelectionMode = false;
  final Set<String> _selectedIds = {};
  Color? _dominantColor;
  bool _showTitleInAppBar = false;
  final ScrollController _scrollController = ScrollController();

  @override
  void initState() {
    super.initState();
    _scrollController.addListener(_onScroll);
    _extractDominantColor();
  }

  @override
  void dispose() {
    _scrollController.removeListener(_onScroll);
    _scrollController.dispose();
    super.dispose();
  }

  void _onScroll() {
    final shouldShow = _scrollController.offset > 280;
    if (shouldShow != _showTitleInAppBar) {
      setState(() => _showTitleInAppBar = shouldShow);
    }
  }

  Future<void> _extractDominantColor() async {
    if (widget.coverUrl == null || widget.coverUrl!.isEmpty) return;
    
    // Only use network images for palette extraction
    final isNetworkUrl = widget.coverUrl!.startsWith('http://') || 
                         widget.coverUrl!.startsWith('https://');
    if (!isNetworkUrl) return;
    
    try {
      final paletteGenerator = await PaletteGenerator.fromImageProvider(
        CachedNetworkImageProvider(widget.coverUrl!),
        maximumColorCount: 16,
      );
      if (mounted) {
        setState(() {
          _dominantColor = paletteGenerator.dominantColor?.color ??
              paletteGenerator.vibrantColor?.color ??
              paletteGenerator.mutedColor?.color;
        });
      }
    } catch (_) {
      // Ignore palette extraction errors
    }
  }

  /// Get tracks for this album from history provider (reactive)
  List<DownloadHistoryItem> _getAlbumTracks(List<DownloadHistoryItem> allItems) {
    return allItems.where((item) {
      // Use albumArtist if available and not empty, otherwise artistName
      final itemArtist = (item.albumArtist != null && item.albumArtist!.isNotEmpty) 
          ? item.albumArtist! 
          : item.artistName;
      final itemKey = '${item.albumName}|$itemArtist';
      final albumKey = '${widget.albumName}|${widget.artistName}';
      return itemKey == albumKey;
    }).toList()
      ..sort((a, b) {
        // Sort by disc number first, then by track number
        final aDisc = a.discNumber ?? 1;
        final bDisc = b.discNumber ?? 1;
        if (aDisc != bDisc) return aDisc.compareTo(bDisc);
        final aNum = a.trackNumber ?? 999;
        final bNum = b.trackNumber ?? 999;
        if (aNum != bNum) return aNum.compareTo(bNum);
        return a.trackName.compareTo(b.trackName);
      });
  }

  /// Get unique disc numbers from tracks (sorted)
  List<int> _getDiscNumbers(List<DownloadHistoryItem> tracks) {
    final discNumbers = tracks
        .map((t) => t.discNumber ?? 1)
        .toSet()
        .toList()
      ..sort();
    return discNumbers;
  }

  /// Check if album has multiple discs
  bool _hasMultipleDiscs(List<DownloadHistoryItem> tracks) {
    return _getDiscNumbers(tracks).length > 1;
  }

  /// Get tracks for a specific disc
  List<DownloadHistoryItem> _getTracksForDisc(List<DownloadHistoryItem> tracks, int discNumber) {
    return tracks.where((t) => (t.discNumber ?? 1) == discNumber).toList();
  }

  void _enterSelectionMode(String itemId) {
    HapticFeedback.mediumImpact();
    setState(() {
      _isSelectionMode = true;
      _selectedIds.add(itemId);
    });
  }

  void _exitSelectionMode() {
    setState(() {
      _isSelectionMode = false;
      _selectedIds.clear();
    });
  }

  void _toggleSelection(String itemId) {
    setState(() {
      if (_selectedIds.contains(itemId)) {
        _selectedIds.remove(itemId);
        if (_selectedIds.isEmpty) {
          _isSelectionMode = false;
        }
      } else {
        _selectedIds.add(itemId);
      }
    });
  }

  void _selectAll(List<DownloadHistoryItem> tracks) {
    setState(() {
      _selectedIds.addAll(tracks.map((e) => e.id));
    });
  }

  Future<void> _deleteSelected(List<DownloadHistoryItem> currentTracks) async {
    final count = _selectedIds.length;
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: Text(context.l10n.downloadedAlbumDeleteSelected),
        content: Text(context.l10n.downloadedAlbumDeleteMessage(count)),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx, false),
            child: Text(context.l10n.dialogCancel),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(ctx, true),
            style: FilledButton.styleFrom(
              backgroundColor: Theme.of(context).colorScheme.error,
            ),
            child: Text(context.l10n.dialogDelete),
          ),
        ],
      ),
    );

    if (confirmed == true && mounted) {
      final historyNotifier = ref.read(downloadHistoryProvider.notifier);
      final idsToDelete = _selectedIds.toList();
      
      int deletedCount = 0;
      for (final id in idsToDelete) {
        final item = currentTracks.where((e) => e.id == id).firstOrNull;
        if (item != null) {
          try {
            final file = File(item.filePath);
            if (await file.exists()) {
              await file.delete();
            }
          } catch (_) {}
          historyNotifier.removeFromHistory(id);
          deletedCount++;
        }
      }
      
      _exitSelectionMode();
      
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(context.l10n.snackbarDeletedTracks(deletedCount))),
        );
      }
    }
  }

  Future<void> _openFile(String filePath) async {
    try {
      final mimeType = audioMimeTypeForPath(filePath);
      await OpenFilex.open(filePath, type: mimeType);
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text(context.l10n.snackbarCannotOpenFile(e.toString()))),
        );
      }
    }
  }

  void _navigateToMetadataScreen(DownloadHistoryItem item) {
    Navigator.push(context, PageRouteBuilder(
      transitionDuration: const Duration(milliseconds: 300),
      reverseTransitionDuration: const Duration(milliseconds: 250),
      pageBuilder: (context, animation, secondaryAnimation) => TrackMetadataScreen(item: item),
      transitionsBuilder: (context, animation, secondaryAnimation, child) => FadeTransition(opacity: animation, child: child),
    ));
  }

  @override
  Widget build(BuildContext context) {
    final colorScheme = Theme.of(context).colorScheme;
    final bottomPadding = MediaQuery.of(context).padding.bottom;
    
    final allHistoryItems = ref.watch(downloadHistoryProvider.select((s) => s.items));
    final tracks = _getAlbumTracks(allHistoryItems);
    
    // Show empty state if no tracks found
    if (tracks.isEmpty) {
      return Scaffold(
        appBar: AppBar(
          title: Text(widget.albumName),
        ),
        body: Center(
          child: Text('No tracks found for this album'),
        ),
      );
    }
    
    final validIds = tracks.map((t) => t.id).toSet();
    _selectedIds.removeWhere((id) => !validIds.contains(id));
    if (_selectedIds.isEmpty && _isSelectionMode) {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (mounted) setState(() => _isSelectionMode = false);
      });
    }

    return PopScope(
      canPop: !_isSelectionMode,
      onPopInvokedWithResult: (didPop, result) {
        if (!didPop && _isSelectionMode) {
          _exitSelectionMode();
        }
      },
      child: Scaffold(
        body: Stack(
          children: [
            CustomScrollView(
              controller: _scrollController,
              slivers: [
                _buildAppBar(context, colorScheme),
                _buildInfoCard(context, colorScheme, tracks),
                _buildTrackListHeader(context, colorScheme, tracks),
                _buildTrackList(context, colorScheme, tracks),
                SliverToBoxAdapter(child: SizedBox(height: _isSelectionMode ? 120 : 32)),
              ],
            ),
            
            AnimatedPositioned(
              duration: const Duration(milliseconds: 250),
              curve: Curves.easeOutCubic,
              left: 0,
              right: 0,
              bottom: _isSelectionMode ? 0 : -(200 + bottomPadding),
              child: _buildSelectionBottomBar(context, colorScheme, tracks, bottomPadding),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildAppBar(BuildContext context, ColorScheme colorScheme) {
    final screenWidth = MediaQuery.of(context).size.width;
    final coverSize = screenWidth * 0.5; // 50% of screen width
    final bgColor = _dominantColor ?? colorScheme.surface;
    
    return SliverAppBar(
      expandedHeight: 320,
      pinned: true,
      stretch: true,
      backgroundColor: colorScheme.surface, // Use theme color for collapsed state
      surfaceTintColor: Colors.transparent,
      title: AnimatedOpacity(
        duration: const Duration(milliseconds: 200),
        opacity: _showTitleInAppBar ? 1.0 : 0.0,
        child: Text(
          widget.albumName,
          style: TextStyle(
            color: colorScheme.onSurface,
            fontWeight: FontWeight.w600,
            fontSize: 16,
          ),
          maxLines: 1,
          overflow: TextOverflow.ellipsis,
        ),
      ),
      flexibleSpace: LayoutBuilder(
        builder: (context, constraints) {
          final collapseRatio = (constraints.maxHeight - kToolbarHeight) / (320 - kToolbarHeight);
          final showContent = collapseRatio > 0.3;
          
          return FlexibleSpaceBar(
            collapseMode: CollapseMode.pin,
            background: Stack(
              fit: StackFit.expand,
              children: [
                // Background with dominant color
                AnimatedContainer(
                  duration: const Duration(milliseconds: 500),
                  decoration: BoxDecoration(
                    gradient: LinearGradient(
                      begin: Alignment.topCenter,
                      end: Alignment.bottomCenter,
                      colors: [
                        bgColor,
                        bgColor.withValues(alpha: 0.8),
                        colorScheme.surface,
                      ],
                      stops: const [0.0, 0.6, 1.0],
                    ),
                  ),
                ),
                // Cover image centered - fade out when collapsing
                AnimatedOpacity(
                  duration: const Duration(milliseconds: 150),
                  opacity: showContent ? 1.0 : 0.0,
                  child: Center(
                    child: Padding(
                      padding: const EdgeInsets.only(top: 60),
                      child: Container(
                        width: coverSize,
                        height: coverSize,
                        decoration: BoxDecoration(
                          borderRadius: BorderRadius.circular(20),
                          boxShadow: [
                            BoxShadow(
                              color: Colors.black.withValues(alpha: 0.4),
                              blurRadius: 30,
                              offset: const Offset(0, 15),
                            ),
                          ],
                        ),
                        child: ClipRRect(
                          borderRadius: BorderRadius.circular(20),
                          child: widget.coverUrl != null
                              ? CachedNetworkImage(
                                  imageUrl: widget.coverUrl!, 
                                  fit: BoxFit.cover, 
                                  memCacheWidth: (coverSize * 2).toInt(),
                                )
                              : Container(
                                  color: colorScheme.surfaceContainerHighest,
                                  child: Icon(Icons.album, size: 64, color: colorScheme.onSurfaceVariant),
                                ),
                        ),
                      ),
                    ),
                  ),
                ),
              ],
            ),
            stretchModes: const [StretchMode.zoomBackground, StretchMode.blurBackground],
          );
        },
      ),
      leading: IconButton(
        icon: Container(
          padding: const EdgeInsets.all(8),
          decoration: BoxDecoration(color: colorScheme.surface.withValues(alpha: 0.8), shape: BoxShape.circle),
          child: Icon(Icons.arrow_back, color: colorScheme.onSurface),
        ),
        onPressed: () => Navigator.pop(context),
      ),
    );
  }

  Widget _buildInfoCard(BuildContext context, ColorScheme colorScheme, List<DownloadHistoryItem> tracks) {
    return SliverToBoxAdapter(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Card(
          elevation: 0,
          color: colorScheme.surfaceContainerLow,
          shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(20)),
          child: Padding(
            padding: const EdgeInsets.all(20),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  widget.albumName,
                  style: Theme.of(context).textTheme.headlineSmall?.copyWith(fontWeight: FontWeight.bold, color: colorScheme.onSurface),
                ),
                const SizedBox(height: 4),
                Text(
                  widget.artistName,
                  style: Theme.of(context).textTheme.bodyLarge?.copyWith(color: colorScheme.onSurfaceVariant),
                ),
                const SizedBox(height: 12),
                Row(
                  children: [
                    Container(
                      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
                      decoration: BoxDecoration(color: colorScheme.primaryContainer, borderRadius: BorderRadius.circular(20)),
                      child: Row(
                        mainAxisSize: MainAxisSize.min,
                        children: [
                          Icon(Icons.download_done, size: 14, color: colorScheme.onPrimaryContainer),
                          const SizedBox(width: 4),
                          Text(context.l10n.downloadedAlbumDownloadedCount(tracks.length), style: TextStyle(color: colorScheme.onPrimaryContainer, fontWeight: FontWeight.w600, fontSize: 12)),
                        ],
                      ),
                    ),
                    const SizedBox(width: 8),
                    if (_getCommonQuality(tracks) != null)
                      Container(
                        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
                        decoration: BoxDecoration(
                          color: _getCommonQuality(tracks)!.startsWith('24') 
                              ? colorScheme.tertiaryContainer 
                              : colorScheme.surfaceContainerHighest,
                          borderRadius: BorderRadius.circular(20),
                        ),
                        child: Text(
                          _getCommonQuality(tracks)!,
                          style: TextStyle(
                            color: _getCommonQuality(tracks)!.startsWith('24') 
                                ? colorScheme.onTertiaryContainer 
                                : colorScheme.onSurfaceVariant,
                            fontWeight: FontWeight.w600,
                            fontSize: 12,
                          ),
                        ),
                      ),
                  ],
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }

  String? _getCommonQuality(List<DownloadHistoryItem> tracks) {
    if (tracks.isEmpty) return null;
    final firstQuality = tracks.first.quality;
    if (firstQuality == null) return null;
    for (final track in tracks) {
      if (track.quality != firstQuality) return null;
    }
    return firstQuality;
  }

  Widget _buildTrackListHeader(BuildContext context, ColorScheme colorScheme, List<DownloadHistoryItem> tracks) {
    return SliverToBoxAdapter(
      child: Padding(
        padding: const EdgeInsets.fromLTRB(20, 8, 20, 8),
        child: Row(
          children: [
            Icon(Icons.queue_music, size: 20, color: colorScheme.primary),
            const SizedBox(width: 8),
            Text(context.l10n.downloadedAlbumTracksHeader, style: Theme.of(context).textTheme.titleMedium?.copyWith(fontWeight: FontWeight.w600, color: colorScheme.onSurface)),
            const Spacer(),
            if (!_isSelectionMode)
              TextButton.icon(
                onPressed: tracks.isNotEmpty ? () => _enterSelectionMode(tracks.first.id) : null,
                icon: const Icon(Icons.checklist, size: 18),
                label: Text(context.l10n.actionSelect),
                style: TextButton.styleFrom(visualDensity: VisualDensity.compact),
              ),
          ],
        ),
      ),
    );
  }

  Widget _buildTrackList(BuildContext context, ColorScheme colorScheme, List<DownloadHistoryItem> tracks) {
    // Check if album has multiple discs
    if (!_hasMultipleDiscs(tracks)) {
      // Single disc - use simple list
      return SliverList(
        delegate: SliverChildBuilderDelegate(
          (context, index) {
            final track = tracks[index];
            return KeyedSubtree(
              key: ValueKey(track.id),
              child: _buildTrackItem(context, colorScheme, track),
            );
          },
          childCount: tracks.length,
        ),
      );
    }

    // Multiple discs - build list with separators
    final discNumbers = _getDiscNumbers(tracks);
    final List<Widget> children = [];

    for (final discNumber in discNumbers) {
      final discTracks = _getTracksForDisc(tracks, discNumber);
      if (discTracks.isEmpty) continue;

      // Add disc separator
      children.add(_buildDiscSeparator(context, colorScheme, discNumber));

      // Add tracks for this disc
      for (final track in discTracks) {
        children.add(
          KeyedSubtree(
            key: ValueKey(track.id),
            child: _buildTrackItem(context, colorScheme, track),
          ),
        );
      }
    }

    return SliverList(
      delegate: SliverChildListDelegate(children),
    );
  }

  Widget _buildDiscSeparator(BuildContext context, ColorScheme colorScheme, int discNumber) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(20, 16, 20, 8),
      child: Row(
        children: [
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
            decoration: BoxDecoration(
              color: colorScheme.secondaryContainer,
              borderRadius: BorderRadius.circular(16),
            ),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(Icons.album, size: 16, color: colorScheme.onSecondaryContainer),
                const SizedBox(width: 6),
                Text(
                  context.l10n.downloadedAlbumDiscHeader(discNumber),
                  style: Theme.of(context).textTheme.labelLarge?.copyWith(
                    color: colorScheme.onSecondaryContainer,
                    fontWeight: FontWeight.w600,
                  ),
                ),
              ],
            ),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Container(
              height: 1,
              color: colorScheme.outlineVariant.withValues(alpha: 0.5),
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildTrackItem(BuildContext context, ColorScheme colorScheme, DownloadHistoryItem track) {
    final isSelected = _selectedIds.contains(track.id);
    
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 8),
      child: Card(
        elevation: 0,
        color: isSelected ? colorScheme.primaryContainer.withValues(alpha: 0.3) : Colors.transparent,
        margin: const EdgeInsets.symmetric(vertical: 2),
        child: ListTile(
          shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
          onTap: _isSelectionMode 
              ? () => _toggleSelection(track.id)
              : () => _navigateToMetadataScreen(track),
          onLongPress: _isSelectionMode ? null : () => _enterSelectionMode(track.id),
          leading: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              if (_isSelectionMode) ...[
                Container(
                  width: 24,
                  height: 24,
                  decoration: BoxDecoration(
                    color: isSelected ? colorScheme.primary : Colors.transparent,
                    shape: BoxShape.circle,
                    border: Border.all(color: isSelected ? colorScheme.primary : colorScheme.outline, width: 2),
                  ),
                  child: isSelected 
                      ? Icon(Icons.check, color: colorScheme.onPrimary, size: 16)
                      : null,
                ),
                const SizedBox(width: 12),
              ],
              SizedBox(
                width: 24,
                child: Text(
                  track.trackNumber?.toString() ?? '-',
                  style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                    color: colorScheme.onSurfaceVariant,
                    fontWeight: FontWeight.w500,
                  ),
                  textAlign: TextAlign.center,
                ),
              ),
            ],
          ),
          title: Text(
            track.trackName,
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
            style: Theme.of(context).textTheme.bodyLarge?.copyWith(fontWeight: FontWeight.w500),
          ),
          subtitle: Text(
            track.artistName,
            maxLines: 1,
            overflow: TextOverflow.ellipsis,
            style: TextStyle(color: colorScheme.onSurfaceVariant),
          ),
          trailing: _isSelectionMode ? null : IconButton(
            onPressed: () => _openFile(track.filePath),
            icon: Icon(Icons.play_arrow, color: colorScheme.primary),
            style: IconButton.styleFrom(
              backgroundColor: colorScheme.primaryContainer.withValues(alpha: 0.3),
            ),
          ),
        ),
      ),
    );
  }

  Widget _buildSelectionBottomBar(BuildContext context, ColorScheme colorScheme, List<DownloadHistoryItem> tracks, double bottomPadding) {
    final selectedCount = _selectedIds.length;
    final allSelected = selectedCount == tracks.length && tracks.isNotEmpty;
    
    return Container(
      decoration: BoxDecoration(
        color: colorScheme.surfaceContainerHigh,
        borderRadius: const BorderRadius.vertical(top: Radius.circular(28)),
        boxShadow: [
          BoxShadow(
            color: Colors.black.withValues(alpha: 0.15),
            blurRadius: 12,
            offset: const Offset(0, -4),
          ),
        ],
      ),
      child: SafeArea(
        top: false,
        child: Padding(
          padding: EdgeInsets.fromLTRB(16, 16, 16, bottomPadding > 0 ? 8 : 16),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Container(
                width: 32,
                height: 4,
                margin: const EdgeInsets.only(bottom: 16),
                decoration: BoxDecoration(
                  color: colorScheme.outlineVariant,
                  borderRadius: BorderRadius.circular(2),
                ),
              ),
              Row(
                children: [
                  IconButton.filledTonal(
                    onPressed: _exitSelectionMode,
                    icon: const Icon(Icons.close),
                    style: IconButton.styleFrom(
                      backgroundColor: colorScheme.surfaceContainerHighest,
                    ),
                  ),
                  const SizedBox(width: 12),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          context.l10n.downloadedAlbumSelectedCount(selectedCount),
                          style: Theme.of(context).textTheme.titleMedium?.copyWith(fontWeight: FontWeight.bold),
                        ),
                        Text(
                          allSelected ? context.l10n.downloadedAlbumAllSelected : context.l10n.downloadedAlbumTapToSelect,
                          style: Theme.of(context).textTheme.bodySmall?.copyWith(color: colorScheme.onSurfaceVariant),
                        ),
                      ],
                    ),
                  ),
                  TextButton.icon(
                    onPressed: () {
                      if (allSelected) {
                        _exitSelectionMode();
                      } else {
                        _selectAll(tracks);
                      }
                    },
                    icon: Icon(allSelected ? Icons.deselect : Icons.select_all, size: 20),
                    label: Text(allSelected ? context.l10n.actionDeselect : context.l10n.actionSelectAll),
                    style: TextButton.styleFrom(foregroundColor: colorScheme.primary),
                  ),
                ],
              ),
              const SizedBox(height: 16),
              SizedBox(
                width: double.infinity,
                child: FilledButton.icon(
                  onPressed: selectedCount > 0 ? () => _deleteSelected(tracks) : null,
                  icon: const Icon(Icons.delete_outline),
                  label: Text(
                    selectedCount > 0 
                        ? context.l10n.downloadedAlbumDeleteCount(selectedCount)
                        : context.l10n.downloadedAlbumSelectToDelete,
                  ),
                  style: FilledButton.styleFrom(
                    backgroundColor: selectedCount > 0 ? colorScheme.error : colorScheme.surfaceContainerHighest,
                    foregroundColor: selectedCount > 0 ? colorScheme.onError : colorScheme.onSurfaceVariant,
                    padding: const EdgeInsets.symmetric(vertical: 16),
                    shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
                  ),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
