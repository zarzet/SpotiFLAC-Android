import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:spotiflac_android/l10n/l10n.dart';
import 'package:spotiflac_android/models/track.dart';
import 'package:spotiflac_android/providers/library_collections_provider.dart';

Future<void> showAddTrackToPlaylistSheet(
  BuildContext context,
  WidgetRef ref,
  Track track,
) async {
  final notifier = ref.read(libraryCollectionsProvider.notifier);
  final state = ref.read(libraryCollectionsProvider);

  if (!context.mounted) return;

  await showModalBottomSheet<void>(
    context: context,
    showDragHandle: true,
    builder: (sheetContext) {
      final playlists = ref.watch(libraryCollectionsProvider).playlists;
      return SafeArea(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            ListTile(
              leading: const Icon(Icons.playlist_add),
              title: Text(sheetContext.l10n.collectionAddToPlaylist),
              subtitle: Text('${track.name} • ${track.artistName}'),
            ),
            const Divider(height: 1),
            ListTile(
              leading: const Icon(Icons.add_circle_outline),
              title: Text(sheetContext.l10n.collectionCreatePlaylist),
              onTap: () async {
                Navigator.of(sheetContext).pop();
                final name = await _promptPlaylistName(context);
                if (name == null || name.trim().isEmpty || !context.mounted) {
                  return;
                }
                final playlistId = await notifier.createPlaylist(name.trim());
                final added = await notifier.addTrackToPlaylist(
                  playlistId,
                  track,
                );
                if (!context.mounted) return;
                ScaffoldMessenger.of(context).showSnackBar(
                  SnackBar(
                    content: Text(
                      added
                          ? context.l10n.collectionAddedToPlaylist(name.trim())
                          : context.l10n.collectionAlreadyInPlaylist(
                              name.trim(),
                            ),
                    ),
                  ),
                );
              },
            ),
            if (playlists.isEmpty)
              Padding(
                padding: const EdgeInsets.fromLTRB(20, 8, 20, 24),
                child: Text(
                  sheetContext.l10n.collectionNoPlaylistsYet,
                  style: Theme.of(sheetContext).textTheme.bodyMedium?.copyWith(
                    color: Theme.of(sheetContext).colorScheme.onSurfaceVariant,
                  ),
                ),
              )
            else
              ConstrainedBox(
                constraints: const BoxConstraints(maxHeight: 320),
                child: ListView.builder(
                  shrinkWrap: true,
                  itemCount: playlists.length,
                  itemBuilder: (context, index) {
                    final playlist = playlists[index];
                    final alreadyInPlaylist = playlist.containsTrack(track);
                    return ListTile(
                      leading: Icon(
                        alreadyInPlaylist
                            ? Icons.playlist_add_check
                            : Icons.queue_music,
                      ),
                      title: Text(playlist.name),
                      subtitle: Text(
                        context.l10n.collectionPlaylistTracks(
                          playlist.tracks.length,
                        ),
                      ),
                      enabled: !alreadyInPlaylist,
                      onTap: !alreadyInPlaylist
                          ? () async {
                              final added = await notifier.addTrackToPlaylist(
                                playlist.id,
                                track,
                              );
                              if (!context.mounted) return;
                              Navigator.of(sheetContext).pop();
                              ScaffoldMessenger.of(context).showSnackBar(
                                SnackBar(
                                  content: Text(
                                    added
                                        ? context.l10n
                                              .collectionAddedToPlaylist(
                                                playlist.name,
                                              )
                                        : context.l10n
                                              .collectionAlreadyInPlaylist(
                                                playlist.name,
                                              ),
                                  ),
                                ),
                              );
                            }
                          : null,
                    );
                  },
                ),
              ),
            const SizedBox(height: 8),
          ],
        ),
      );
    },
  );

  if (!context.mounted) return;

  final afterState = ref.read(libraryCollectionsProvider);
  if (afterState.playlists.length != state.playlists.length) {
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(content: Text(context.l10n.collectionPlaylistCreated)),
    );
  }
}

Future<String?> _promptPlaylistName(BuildContext context) async {
  final controller = TextEditingController();
  final formKey = GlobalKey<FormState>();

  final result = await showDialog<String>(
    context: context,
    builder: (dialogContext) {
      return AlertDialog(
        title: Text(dialogContext.l10n.collectionCreatePlaylist),
        content: Form(
          key: formKey,
          child: TextFormField(
            controller: controller,
            autofocus: true,
            textInputAction: TextInputAction.done,
            decoration: InputDecoration(
              hintText: dialogContext.l10n.collectionPlaylistNameHint,
            ),
            validator: (value) {
              final trimmed = value?.trim() ?? '';
              if (trimmed.isEmpty) {
                return dialogContext.l10n.collectionPlaylistNameRequired;
              }
              return null;
            },
            onFieldSubmitted: (_) {
              if (formKey.currentState?.validate() != true) return;
              Navigator.of(dialogContext).pop(controller.text.trim());
            },
          ),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(dialogContext).pop(),
            child: Text(dialogContext.l10n.dialogCancel),
          ),
          FilledButton(
            onPressed: () {
              if (formKey.currentState?.validate() != true) return;
              Navigator.of(dialogContext).pop(controller.text.trim());
            },
            child: Text(dialogContext.l10n.actionCreate),
          ),
        ],
      );
    },
  );

  return result;
}
