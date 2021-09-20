import React, { FC, useEffect, useState } from 'react';
import { connect, MapStateToProps } from 'react-redux';
import { NavModel } from '@grafana/data';
import Page from 'app/core/components/Page/Page';
import { StoreState } from 'app/types';
import { GrafanaRouteComponentProps } from '../../core/navigation/types';
import { getNavModel } from 'app/core/selectors/navModel';
import { useAsync } from 'react-use';
import { PlaylistDTO } from './types';
import { ConfirmModal } from '@grafana/ui';
import PageActionBar from 'app/core/components/PageActionBar/PageActionBar';
import EmptyListCTA from '../../core/components/EmptyListCTA/EmptyListCTA';
import { deletePlaylist, getAllPlaylist } from './api';
import { StartModal } from './StartModal';
import { PlaylistPageList } from './PlaylistPageList';
import { EmptyQueryListBanner } from './EmptyQueryListBanner';

interface ConnectedProps {
  navModel: NavModel;
}
export interface PlaylistPageProps extends ConnectedProps, GrafanaRouteComponentProps {}

export const PlaylistPage: FC<PlaylistPageProps> = ({ navModel }) => {
  const [searchQuery, setSearchQuery] = useState('');
  const [hasFetched, setHasFetched] = useState(false);
  const [startPlaylist, setStartPlaylist] = useState<PlaylistDTO | undefined>();
  const [playlistToDelete, setPlaylistToDelete] = useState<PlaylistDTO | undefined>();
  const [forcePlaylistsFetch, setForcePlaylistsFetch] = useState(0);

  const { value: playlists, loading } = useAsync(async () => {
    return getAllPlaylist(searchQuery);
  }, [forcePlaylistsFetch, searchQuery]);
  useEffect(() => {
    if (!hasFetched && !loading) {
      setHasFetched(true);
    }
  }, [loading, hasFetched]);
  const hasPlaylists = playlists && playlists.length > 0;
  const onDismissDelete = () => setPlaylistToDelete(undefined);
  const onDeletePlaylist = () => {
    if (!playlistToDelete) {
      return;
    }
    deletePlaylist(playlistToDelete.id).finally(() => {
      setForcePlaylistsFetch(forcePlaylistsFetch + 1);
      setPlaylistToDelete(undefined);
    });
  };

  const emptyListBanner = (
    <EmptyListCTA
      title="There are no playlists created yet"
      buttonIcon="plus"
      buttonLink="playlists/new"
      buttonTitle="Create Playlist"
      proTip="You can use playlists to cycle dashboards on TVs without user control"
      proTipLink="http://docs.grafana.org/reference/playlist/"
      proTipLinkTitle="Learn more"
      proTipTarget="_blank"
    />
  );

  return (
    <Page navModel={navModel}>
      <Page.Contents isLoading={!hasFetched}>
        <PageActionBar
          searchQuery={searchQuery}
          linkButton={{ title: 'New playlist', href: '/playlists/new' }}
          setSearchQuery={setSearchQuery}
        />
        {!hasPlaylists && searchQuery ? (
          <EmptyQueryListBanner />
        ) : (
          <PlaylistPageList
            playlists={playlists}
            setStartPlaylist={setStartPlaylist}
            setPlaylistToDelete={setPlaylistToDelete}
          />
        )}

        {!hasPlaylists && searchQuery.length < 1 && emptyListBanner}

        {playlistToDelete && (
          <ConfirmModal
            title={playlistToDelete.name}
            confirmText="delete"
            body={`Are you sure you want to delete '${playlistToDelete.name}' playlist?`}
            onConfirm={onDeletePlaylist}
            isOpen={Boolean(playlistToDelete)}
            onDismiss={onDismissDelete}
          />
        )}
        {startPlaylist && <StartModal playlist={startPlaylist} onDismiss={() => setStartPlaylist(undefined)} />}
      </Page.Contents>
    </Page>
  );
};

const mapStateToProps: MapStateToProps<ConnectedProps, {}, StoreState> = (state: StoreState) => ({
  navModel: getNavModel(state.navIndex, 'playlists'),
});

export default connect(mapStateToProps)(PlaylistPage);
